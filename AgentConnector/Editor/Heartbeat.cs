using System;
using System.IO;
using System.Security.Cryptography;
using System.Text;
using Newtonsoft.Json;
using UnityEditor;
using UnityEngine;

namespace HeraAgent
{
    [InitializeOnLoad]
    public static class Heartbeat
    {
        static readonly string s_Dir = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile), ".hera-agent", "instances");

        static double s_LastWrite;
        const double INTERVAL = 0.5;
        const double COMPILE_START_TIMEOUT = 30.0; // hard cap awaiting compile-start after request
        const double FORCE_REWRITE_INTERVAL = 5.0; // re-write at least this often even if content unchanged (staleness probe)
        static string s_ForcedState;
        static double s_CompileRequestTime;
        static bool s_SawCompileStart;
        static string s_FilePath;
        static string s_LastJson;
        static double s_LastForcedWrite;

        static Heartbeat()
        {
            EditorApplication.update += Tick;
            EditorApplication.quitting += Cleanup;
            AssemblyReloadEvents.beforeAssemblyReload += OnBeforeAssemblyReload;
            AssemblyReloadEvents.afterAssemblyReload += () => { s_ForcedState = null; s_LastWrite = 0; };
            EditorApplication.playModeStateChanged += OnPlayModeChanged;
        }

        static void OnBeforeAssemblyReload()
        {
            WriteState("reloading");
        }

        static void OnPlayModeChanged(PlayModeStateChange change)
        {
            if (change == PlayModeStateChange.ExitingEditMode)
                WriteState("entering_playmode");
        }

        static void WriteState(string state)
        {
            s_ForcedState = state;
            Write();
        }

        /// <summary>
        /// Marks that a compile was requested. Keeps "compiling" state forced
        /// for a grace period so the CLI poller never sees a premature "ready".
        /// </summary>
        public static void MarkCompileRequested()
        {
            s_CompileRequestTime = EditorApplication.timeSinceStartup;
            s_SawCompileStart = false;
            WriteState("compiling");
        }

        static void Tick()
        {
            if (HttpServer.Port == 0) return;

            var now = EditorApplication.timeSinceStartup;
            if (now - s_LastWrite < INTERVAL) return;
            s_LastWrite = now;

            if (s_CompileRequestTime > 0)
            {
                if (EditorApplication.isCompiling)
                    s_SawCompileStart = true;

                var elapsed = now - s_CompileRequestTime;
                // Keep "compiling" forced while Unity is actively compiling OR we
                // are still inside the start-up window waiting for compile to
                // actually begin. Without this, a slow compile-start would let
                // a "ready" tick slip out and trick CLI pollers into a premature
                // exec, which then fails against not-yet-rebuilt code.
                var awaitingStart = !s_SawCompileStart && elapsed < COMPILE_START_TIMEOUT;
                if (EditorApplication.isCompiling || awaitingStart)
                {
                    Write();
                    return;
                }
                s_CompileRequestTime = 0;
                s_SawCompileStart = false;
            }

            s_ForcedState = null;
            Write();
        }

        static string GetFilePath()
        {
            if (s_FilePath != null) return s_FilePath;
            var projectPath = Application.dataPath.Replace("/Assets", "");
            using var md5 = MD5.Create();
            var hash = BitConverter.ToString(md5.ComputeHash(Encoding.UTF8.GetBytes(projectPath)))
                .Replace("-", "").Substring(0, 16).ToLower();
            s_FilePath = Path.Combine(s_Dir, $"{hash}.json");
            return s_FilePath;
        }

        static void Write()
        {
            var status = new
            {
                state = s_ForcedState ?? GetState(),
                projectPath = Application.dataPath.Replace("/Assets", ""),
                port = HttpServer.Port,
                pid = System.Diagnostics.Process.GetCurrentProcess().Id,
                unityVersion = Application.unityVersion,
                timestamp = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds(),
                compileErrors = EditorUtility.scriptCompilationFailed,
            };

            var json = JsonConvert.SerializeObject(status);

            // Skip disk I/O when the serialized payload (minus timestamp drift)
            // matches the last write. timestamp is part of the JSON so the cheap
            // compare here only kicks in for the forced-rewrite path below; the
            // primary win is staying off the disk during idle reload cycles.
            var now = EditorApplication.timeSinceStartup;
            var jsonForCompare = StripTimestamp(json);
            var stateUnchanged = jsonForCompare == s_LastJson;
            var forceWrite = now - s_LastForcedWrite >= FORCE_REWRITE_INTERVAL;
            if (stateUnchanged && !forceWrite) return;

            try
            {
                Directory.CreateDirectory(s_Dir);
                var path = GetFilePath();
                var tmp = path + ".tmp";
                File.WriteAllText(tmp, json);
                // Atomic replace: a concurrent reader either sees the full prior
                // file or the full new one, never a partial JSON document.
                if (File.Exists(path))
                    File.Replace(tmp, path, null);
                else
                    File.Move(tmp, path);
                s_LastJson = jsonForCompare;
                s_LastForcedWrite = now;
            }
            catch (Exception ex)
            {
                UnityEngine.Debug.LogError($"[Hera] Heartbeat write failed: {ex.Message}");
            }
        }

        private static string StripTimestamp(string json)
        {
            // crude but cheap: nuke the timestamp field for content comparison
            var idx = json.IndexOf("\"timestamp\":", StringComparison.Ordinal);
            if (idx < 0) return json;
            var end = json.IndexOf(',', idx);
            if (end < 0) end = json.IndexOf('}', idx);
            if (end < 0) return json;
            return json.Substring(0, idx) + json.Substring(end);
        }

        static string GetState()
        {
            if (EditorApplication.isCompiling) return "compiling";
            if (EditorApplication.isUpdating) return "refreshing";
            if (EditorApplication.isPlaying)
                return EditorApplication.isPaused ? "paused" : "playing";
            return "ready";
        }

        public static void Cleanup()
        {
            if (HttpServer.Port == 0) return;
            s_ForcedState = "stopped";
            Write();
        }
    }
}
