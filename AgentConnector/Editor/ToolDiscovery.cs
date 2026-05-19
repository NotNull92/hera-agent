using System;
using System.Collections.Generic;
using System.Linq;
using System.Reflection;
using Newtonsoft.Json.Linq;

namespace HeraAgent
{
    /// <summary>
    /// Finds [HeraTool] handlers on demand via reflection.
    /// No caching, no registration — every call scans live.
    /// </summary>
    public static class ToolDiscovery
    {
        public static MethodInfo FindHandler(string command)
        {
            MethodInfo found = null;
            Type foundType = null;

            foreach (var assembly in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = assembly.GetTypes(); }
                catch (ReflectionTypeLoadException) { continue; }

                foreach (var type in types)
                {
                    if (type.IsClass == false) continue;
                    var attr = type.GetCustomAttribute<HeraToolAttribute>();
                    if (attr == null) continue;

                    var name = attr.Name ?? StringCaseUtility.ToSnakeCase(type.Name);
                    if (name != command) continue;

                    // Check for static method first (traditional tools)
                    var staticMethod = type.GetMethod("HandleCommand",
                        BindingFlags.Public | BindingFlags.Static, null,
                        new[] { typeof(JObject) }, null);

                    // Check for instance method (class-based tools)
                    var instanceMethod = type.GetMethod("HandleCommand",
                        BindingFlags.Public | BindingFlags.Instance, null,
                        new[] { typeof(JObject) }, null);

                    // Prefer static method if both exist
                    var method = staticMethod ?? instanceMethod;

                    if (method == null) continue;

                    if (found != null)
                    {
                        UnityEngine.Debug.LogError(
                            $"[Hera] Duplicate tool '{command}': " +
                            $"{foundType.FullName} and {type.FullName}. Using first found.");
                        continue;
                    }

                    found = method;
                    foundType = type;
                }
            }

            return found;
        }

        /// <summary>
        /// Slim default for agent consumers: name, description, schema only.
        /// Token-cost of `list` was ~90% redundancy across parameters/schema/metadata.
        /// For full per-tool detail use GetToolSchema(name).
        /// </summary>
        public static List<object> GetToolSchemas()
        {
            var tools = new List<object>();
            foreach (var (name, type, attr) in EnumerateTools())
            {
                var paramsType = type.GetNestedType("Parameters");
                tools.Add(new
                {
                    name,
                    description = attr.Description ?? "",
                    schema = GetToolMetadata(type)?.ParametersSchema
                        ?? GetLegacyParameterSchema(paramsType),
                });
            }
            return tools;
        }

        /// <summary>
        /// Names-only listing — one entry per tool. Use when an agent just needs
        /// to know what's available before picking one to introspect.
        /// </summary>
        public static List<object> GetToolNames()
        {
            var tools = new List<object>();
            foreach (var (name, _, attr) in EnumerateTools())
            {
                tools.Add(new { name, description = attr.Description ?? "" });
            }
            return tools;
        }

        /// <summary>
        /// Full schema for a single tool — returned by `list --tool &lt;name&gt;`.
        /// Includes parameters schema, output schema, and metadata. Null if not found.
        /// </summary>
        public static object GetToolSchema(string toolName)
        {
            foreach (var (name, type, attr) in EnumerateTools())
            {
                if (name != toolName) continue;
                var paramsType = type.GetNestedType("Parameters");
                return new
                {
                    name,
                    description = attr.Description ?? "",
                    group = attr.Group ?? "",
                    groups = attr.Groups ?? new string[0],
                    schema = GetToolMetadata(type)?.ParametersSchema
                        ?? GetLegacyParameterSchema(paramsType),
                    output_schema = GetToolMetadata(type)?.OutputSchema
                        ?? GetDefaultOutputSchema(),
                    metadata = new
                    {
                        enum_support = HasEnumSupport(paramsType),
                        default_support = HasDefaultSupport(paramsType),
                        output_schema_support = HasOutputSchemaSupport(paramsType),
                        custom_types = GetCustomTypes(paramsType),
                    },
                };
            }
            return null;
        }

        private static IEnumerable<(string name, Type type, HeraToolAttribute attr)> EnumerateTools()
        {
            var seen = new Dictionary<string, Type>();
            foreach (var assembly in AppDomain.CurrentDomain.GetAssemblies())
            {
                Type[] types;
                try { types = assembly.GetTypes(); }
                catch (ReflectionTypeLoadException) { continue; }

                foreach (var type in types)
                {
                    if (!type.IsClass) continue;
                    var attr = type.GetCustomAttribute<HeraToolAttribute>();
                    if (attr == null) continue;
                    if (!attr.Enabled) continue;

                    var name = attr.Name ?? StringCaseUtility.ToSnakeCase(type.Name);
                    if (seen.TryGetValue(name, out var existing))
                    {
                        UnityEngine.Debug.LogError(
                            $"[Hera] Duplicate tool name '{name}': " +
                            $"{existing.FullName} and {type.FullName}. " +
                            $"Rename one or remove the duplicate.");
                        continue;
                    }
                    seen[name] = type;
                    yield return (name, type, attr);
                }
            }
        }

        private static ToolMetadata GetToolMetadata(Type toolType)
        {
            try
            {
                var attr = toolType.GetCustomAttribute<HeraToolAttribute>();
                var toolName = attr?.Name ?? StringCaseUtility.ToSnakeCase(toolType.Name);
                ToolMetadataRegistry.Register(toolType);
                return ToolMetadataRegistry.GetTool(toolName);
            }
            catch
            {
                return null;
            }
        }

        private static JObject GetLegacyParameterSchema(Type paramsType)
        {
            if (paramsType == null) return null;

            var schema = new JObject
            {
                ["type"] = "object",
                ["properties"] = new JObject()
            };

            var requiredParams = new List<string>();

            foreach (var prop in paramsType.GetProperties())
            {
                var attr = prop.GetCustomAttribute<ToolParameterAttribute>();
                if (attr == null) continue;

                var propName = StringCaseUtility.ToSnakeCase(prop.Name);
                var paramSchema = new JObject
                {
                    ["type"] = GetTypeName(prop.PropertyType),
                    ["description"] = attr.Description ?? ""
                };

                if (attr.Required)
                {
                    requiredParams.Add(propName);
                }

                schema["properties"][propName] = paramSchema;
            }

            if (requiredParams.Count > 0)
            {
                schema["required"] = new JArray(requiredParams);
            }

            return schema;
        }

        private static string GetTypeName(Type type)
        {
            if (type == typeof(string)) return "string";
            if (type == typeof(int) || type == typeof(int?)) return "integer";
            if (type == typeof(float) || type == typeof(float?)) return "number";
            if (type == typeof(bool) || type == typeof(bool?)) return "boolean";
            if (type.IsArray) return "array";
            return "string";
        }

        private static bool HasEnumSupport(Type paramsType)
        {
            if (paramsType == null) return false;

            return paramsType.GetProperties()
                .Any(p => p.GetCustomAttribute<ToolParameterAttribute>()?.EnumType != null);
        }

        private static bool HasDefaultSupport(Type paramsType)
        {
            if (paramsType == null) return false;

            return paramsType.GetProperties()
                .Any(p => p.GetCustomAttribute<ToolParameterAttribute>()?.Default != null);
        }

        private static List<string> GetCustomTypes(Type paramsType)
        {
            if (paramsType == null) return new List<string>();

            return paramsType.GetProperties()
                .Where(p => p.GetCustomAttribute<ToolParameterAttribute>()?.EnumType != null)
                .Select(p => p.GetCustomAttribute<ToolParameterAttribute>().EnumType)
                .Where(type => !string.IsNullOrEmpty(type))
                .Distinct()
                .ToList();
        }

        private static JObject GetDefaultOutputSchema()
        {
            return new JObject
            {
                ["type"] = "object",
                ["properties"] = new JObject
                {
                    ["success"] = new JObject { ["type"] = "boolean", ["description"] = "Whether the operation succeeded" },
                    ["message"] = new JObject { ["type"] = "string", ["description"] = "Success or error message" },
                    ["data"] = new JObject { ["type"] = "object", ["description"] = "Tool-specific output data" }
                }
            };
        }

        private static bool HasOutputSchemaSupport(Type paramsType)
        {
            if (paramsType == null) return false;

            return paramsType.GetProperties()
                .Any(p => p.GetCustomAttribute<ToolParameterAttribute>()?.OutputSchema != null);
        }
    }
}
