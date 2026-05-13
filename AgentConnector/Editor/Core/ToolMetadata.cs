using System;
using System.Collections.Generic;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace HeraAgent
{
    /// <summary>
    /// Represents metadata for a tool parameter including schema information
    /// </summary>
    public class ToolParameterMetadata
    {
        public string Name { get; set; }
        public string Description { get; set; }
        public bool Required { get; set; }
        public string DefaultValue { get; set; }
        public string EnumType { get; set; }
        public string Default { get; set; }
        public string Type { get; set; }
        public string OutputSchema { get; set; }
        public JObject Schema { get; set; }

        public ToolParameterMetadata(ToolParameterAttribute attr, Type propertyType)
        {
            Name = attr.Name;
            Description = attr.Description;
            Required = attr.Required;
            DefaultValue = attr.DefaultValue;
            EnumType = attr.EnumType;
            Default = attr.Default;
            Type = GetTypeName(propertyType);
            OutputSchema = attr.OutputSchema;
            Schema = GenerateSchema(attr, propertyType);
        }

        private string GetTypeName(Type type)
        {
            if (type == typeof(string)) return "string";
            if (type == typeof(int) || type == typeof(int?)) return "integer";
            if (type == typeof(float) || type == typeof(float?)) return "number";
            if (type == typeof(bool) || type == typeof(bool?)) return "boolean";
            if (type.IsArray) return "array";
            return "string";
        }

        private JObject GenerateSchema(ToolParameterAttribute attr, Type propertyType)
        {
            var schema = new JObject
            {
                ["type"] = GetTypeName(propertyType)
            };

            if (!string.IsNullOrEmpty(attr.Description))
            {
                schema["description"] = attr.Description;
            }

            if (attr.Required)
            {
                schema["required"] = true;
            }

            if (!string.IsNullOrEmpty(attr.Default))
            {
                schema["default"] = ConvertDefaultValue(attr.Default, propertyType);
            }
            else if (!string.IsNullOrEmpty(attr.DefaultValue))
            {
                schema["default"] = attr.DefaultValue;
            }

            if (!string.IsNullOrEmpty(attr.EnumType))
            {
                var enumValues = GetEnumValues(attr.EnumType);
                if (enumValues != null && enumValues.Count > 0)
                {
                    schema["enum"] = new JArray(enumValues);
                }
            }

            return schema;
        }

        private JToken ConvertDefaultValue(string defaultValue, Type type)
        {
            try
            {
                if (type == typeof(string)) return defaultValue;
                if (type == typeof(int)) return int.Parse(defaultValue);
                if (type == typeof(float)) return float.Parse(defaultValue);
                if (type == typeof(bool)) return bool.Parse(defaultValue);
                return defaultValue;
            }
            catch
            {
                return defaultValue;
            }
        }

        private List<string> GetEnumValues(string enumName)
        {
            try
            {
                foreach (var assembly in System.AppDomain.CurrentDomain.GetAssemblies())
                {
                    try
                    {
                        var enumType = assembly.GetType(enumName);
                        if (enumType != null && enumType.IsEnum)
                            return System.Enum.GetNames(enumType).ToList();
                    }
                    catch { }
                }
            }
            catch
            {
                // Enum not found, ignore
            }
            return null;
        }
    }

    /// <summary>
    /// Represents metadata for a CLI tool including parameter schemas
    /// </summary>
    public class ToolMetadata
    {
        public string Name { get; set; }
        public string Description { get; set; }
        public string Group { get; set; }
        public string[] Groups { get; set; }
        public JObject ParametersSchema { get; set; }
        public JObject OutputSchema { get; set; }
        public List<ToolParameterMetadata> Parameters { get; set; }
        public bool IsBuiltIn { get; set; }
        public bool Enabled { get; set; }

        public ToolMetadata(Type toolType, bool isBuiltIn = false)
        {
            IsBuiltIn = isBuiltIn;
            
            var toolAttr = toolType.GetCustomAttributes(typeof(HeraToolAttribute), false)
                .FirstOrDefault() as HeraToolAttribute;
            
            Name = toolAttr?.Name ?? GetSnakeCaseName(toolType.Name);
            Description = toolAttr?.Description ?? "";
            Group = toolAttr?.Group ?? "";
            Groups = toolAttr?.Groups ?? Array.Empty<string>();
            Enabled = toolAttr?.Enabled ?? true;

            var parameters = toolType.GetProperties()
                .Where(p => p.GetCustomAttributes(typeof(ToolParameterAttribute), false).Length > 0)
                .Select(p => new ToolParameterMetadata(
                    p.GetCustomAttributes(typeof(ToolParameterAttribute), false).First() as ToolParameterAttribute,
                    p.PropertyType))
                .ToList();

            Parameters = parameters;
            ParametersSchema = GenerateParametersSchema(parameters);
            OutputSchema = GenerateOutputSchema(parameters);
        }

        private JObject GenerateOutputSchema(List<ToolParameterMetadata> parameters)
        {
            var hasOutputSchema = parameters.Any(p => !string.IsNullOrEmpty(p.OutputSchema));
            
            if (!hasOutputSchema)
            {
                // Default output schema
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

            // Use custom output schema if provided
            var customOutputSchema = parameters.FirstOrDefault(p => !string.IsNullOrEmpty(p.OutputSchema));
            if (customOutputSchema != null)
            {
                try
                {
                    return JObject.Parse(customOutputSchema.OutputSchema);
                }
                catch
                {
                    return new JObject
                    {
                        ["type"] = "object",
                        ["properties"] = new JObject
                        {
                            ["success"] = new JObject { ["type"] = "boolean" },
                            ["message"] = new JObject { ["type"] = "string" }
                        }
                    };
                }
            }

            return new JObject { ["type"] = "object" };
        }

        private string GetSnakeCaseName(string className)
        {
            return System.Text.RegularExpressions.Regex.Replace(className, "([a-z])([A-Z])", "$1_$2").ToLower();
        }

        private JObject GenerateParametersSchema(List<ToolParameterMetadata> parameters)
        {
            var schema = new JObject
            {
                ["type"] = "object",
                ["properties"] = new JObject()
            };

            foreach (var param in parameters)
            {
                schema["properties"][param.Name] = param.Schema;
            }

            var requiredParams = parameters.Where(p => p.Required).Select(p => p.Name).ToList();
            if (requiredParams.Count > 0)
            {
                schema["required"] = new JArray(requiredParams);
            }

            return schema;
        }
    }

    /// <summary>
    /// Registry for tool metadata management
    /// </summary>
    public static class ToolMetadataRegistry
    {
        private static readonly Dictionary<string, ToolMetadata> _tools = new Dictionary<string, ToolMetadata>();
        
        public static event Action<string> ToolRegistered;
        public static event Action<string> ToolUnregistered;
        public static event Action ToolsRefreshed;

        public static void Register(Type toolType, bool isBuiltIn = false)
        {
            var metadata = new ToolMetadata(toolType, isBuiltIn);
            _tools[metadata.Name] = metadata;
            
            if (metadata.Enabled)
            {
                ToolRegistered?.Invoke(metadata.Name);
            }
            
            ToolsRefreshed?.Invoke();
        }

        public static void Unregister(string toolName)
        {
            if (_tools.Remove(toolName))
            {
                ToolUnregistered?.Invoke(toolName);
                ToolsRefreshed?.Invoke();
            }
        }

        public static void EnableTool(string toolName)
        {
            if (_tools.TryGetValue(toolName, out var metadata))
            {
                metadata.Enabled = true;
                ToolRegistered?.Invoke(toolName);
                ToolsRefreshed?.Invoke();
            }
        }

        public static void DisableTool(string toolName)
        {
            if (_tools.TryGetValue(toolName, out var metadata))
            {
                metadata.Enabled = false;
                ToolUnregistered?.Invoke(toolName);
                ToolsRefreshed?.Invoke();
            }
        }

        public static void SetToolEnabled(string toolName, bool enabled)
        {
            if (_tools.TryGetValue(toolName, out var metadata))
            {
                var wasEnabled = metadata.Enabled;
                metadata.Enabled = enabled;
                
                if (wasEnabled && !enabled)
                {
                    ToolUnregistered?.Invoke(toolName);
                }
                else if (!wasEnabled && enabled)
                {
                    ToolRegistered?.Invoke(toolName);
                }
                
                ToolsRefreshed?.Invoke();
            }
        }

        public static bool IsToolEnabled(string toolName)
        {
            return _tools.TryGetValue(toolName, out var metadata) && metadata.Enabled;
        }

        public static void Register<T>(bool isBuiltIn = false) where T : class
        {
            Register(typeof(T), isBuiltIn);
        }

        public static void Unregister<T>() where T : class
        {
            var typeName = typeof(T).Name;
            var toolName = System.Text.RegularExpressions.Regex.Replace(typeName, "([a-z])([A-Z])", "$1_$2").ToLower();
            Unregister(toolName);
        }

        public static ToolMetadata GetTool(string toolName)
        {
            _tools.TryGetValue(toolName, out var metadata);
            return metadata;
        }

        public static List<ToolMetadata> GetAllTools()
        {
            return _tools.Values.ToList();
        }

        public static List<ToolMetadata> GetToolsByGroup(string group)
        {
            return _tools.Values.Where(t => t.Group == group).ToList();
        }

        public static List<ToolMetadata> GetToolsByGroups(string[] groups)
        {
            return _tools.Values.Where(t => t.Groups.Any(g => groups.Contains(g))).ToList();
        }

        public static List<string> GetAllGroups()
        {
            return _tools.Values
                .SelectMany(t => t.Groups)
                .Distinct()
                .OrderBy(g => g)
                .ToList();
        }

        public static Dictionary<string, List<ToolMetadata>> GetToolsGroupedByGroups()
        {
            var result = new Dictionary<string, List<ToolMetadata>>();
            
            foreach (var group in GetAllGroups())
            {
                result[group] = GetToolsByGroups(new[] { group });
            }
            
            return result;
        }

        public static List<ToolMetadata> GetBuiltInTools()
        {
            return _tools.Values.Where(t => t.IsBuiltIn).ToList();
        }

        public static List<ToolMetadata> GetCustomTools()
        {
            return _tools.Values.Where(t => !t.IsBuiltIn).ToList();
        }

        public static bool ToolExists(string toolName)
        {
            return _tools.ContainsKey(toolName);
        }

        public static void Refresh()
        {
            ToolsRefreshed?.Invoke();
        }

        public static void Clear()
        {
            _tools.Clear();
            ToolsRefreshed?.Invoke();
        }
    }
}