using System.Collections.Generic;

namespace HeraAgent
{
    public class SuccessResponse
    {
        public bool success = true;
        public string message;
        public object data;
        public Dictionary<string, long> timings;

        public SuccessResponse(string message, object data = null)
        {
            this.message = message;
            this.data = data;
        }
    }

    public class ErrorResponse
    {
        public bool success = false;
        public string message;
        public object data;
        public Dictionary<string, long> timings;

        public ErrorResponse(string message, object data = null)
        {
            this.message = message;
            this.data = data;
        }
    }

    /// <summary>
    /// Helper for attaching timing measurements to tool responses.
    /// Tolerates objects that aren't Success/ErrorResponse — no-op in that case.
    /// </summary>
    public static class ResponseTimings
    {
        public static void Set(object response, string key, long valueMs)
        {
            switch (response)
            {
                case SuccessResponse s:
                    if (s.timings == null) s.timings = new Dictionary<string, long>();
                    s.timings[key] = valueMs;
                    break;
                case ErrorResponse e:
                    if (e.timings == null) e.timings = new Dictionary<string, long>();
                    e.timings[key] = valueMs;
                    break;
            }
        }

        public static void Merge(object response, Dictionary<string, long> source)
        {
            if (source == null) return;
            foreach (var kv in source) Set(response, kv.Key, kv.Value);
        }
    }
}
