package tools

func defaultTools() []Tool {
	return []Tool{
		{
			Name:        "Read",
			Description: "Read a file from the local filesystem with cat -n style line numbers. Use the line numbers when responding so the user can navigate. Default reads up to 2000 lines from the start; pass offset+limit to page through larger files.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Absolute path of the file to read."},
					"offset":    map[string]any{"type": "integer", "description": "Line number to start reading at (1-indexed). Default: 1."},
					"limit":     map[string]any{"type": "integer", "description": "Maximum number of lines to read. Default: 2000."},
				},
				"required":             []string{"file_path"},
				"additionalProperties": false,
			},
			Permission: AutoApprove,
			Run:        runRead,
		},
		{
			Name:        "Write",
			Description: "Overwrite a file on the local filesystem. Creates parent directories as needed. Use Edit to modify existing files.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Absolute path of the file to write."},
					"content":   map[string]any{"type": "string", "description": "Full content of the file."},
				},
				"required":             []string{"file_path", "content"},
				"additionalProperties": false,
			},
			Permission: Prompt,
			Run:        runWrite,
		},
		{
			Name:        "Edit",
			Description: "Replace exact text in a file. old_string must occur exactly once unless replace_all is true. The match is byte-exact; preserve indentation. To rename a symbol throughout, use replace_all.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path":   map[string]any{"type": "string", "description": "Absolute path of the file to edit."},
					"old_string":  map[string]any{"type": "string", "description": "Exact text to replace. Must be unique unless replace_all is true."},
					"new_string":  map[string]any{"type": "string", "description": "Replacement text."},
					"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence (rename). Default: false."},
				},
				"required":             []string{"file_path", "old_string", "new_string"},
				"additionalProperties": false,
			},
			Permission: Prompt,
			Run:        runEdit,
		},
		{
			Name:        "Bash",
			Description: "Execute a shell command via `bash -c`. The user is prompted before each call. Default timeout is 2 minutes (max 10).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":     map[string]any{"type": "string", "description": "The shell command to run."},
					"description": map[string]any{"type": "string", "description": "Short label shown in the permission prompt."},
					"timeout":     map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 120000)."},
				},
				"required":             []string{"command"},
				"additionalProperties": false,
			},
			Permission: Prompt,
			Run:        runBash,
		},
		{
			Name:        "Glob",
			Description: "List files matching a glob pattern (supports ** for recursion). Returns absolute paths sorted by mtime descending. Use this to find files by name; use Grep to search file contents.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Glob pattern, e.g. **/*.go or src/**/test_*.py."},
					"path":    map[string]any{"type": "string", "description": "Absolute root directory. Default: current working directory."},
				},
				"required":             []string{"pattern"},
				"additionalProperties": false,
			},
			Permission: AutoApprove,
			Run:        runGlob,
		},
		{
			Name:        "Grep",
			Description: "Search file contents using a Go-flavored regex. output_mode controls the result shape (files_with_matches | content | count). Skips .git, node_modules, vendor, .venv. Limited to 5MB per file.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":     map[string]any{"type": "string", "description": "Regex (Go RE2 syntax)."},
					"path":        map[string]any{"type": "string", "description": "Root directory to search. Default: cwd."},
					"glob":        map[string]any{"type": "string", "description": "Restrict to files matching this glob, e.g. **/*.go."},
					"output_mode": map[string]any{"type": "string", "enum": []string{"files_with_matches", "content", "count"}, "description": "Default: files_with_matches."},
					"-i":          map[string]any{"type": "boolean", "description": "Case-insensitive."},
					"-n":          map[string]any{"type": "boolean", "description": "Include line numbers (content mode only)."},
					"head_limit":  map[string]any{"type": "integer", "description": "Max entries to return. Default: 200."},
				},
				"required":             []string{"pattern"},
				"additionalProperties": false,
			},
			Permission: AutoApprove,
			Run:        runGrep,
		},
		{
			Name:        "WebFetch",
			Description: "Download a URL (http/https) and return up to 200KB of the response body, prefixed by status and content-type. The user is prompted before each fetch. Use this for docs, READMEs, public APIs.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":    map[string]any{"type": "string", "description": "Absolute http(s) URL."},
					"prompt": map[string]any{"type": "string", "description": "Ignored locally — kept for parity with Anthropic's WebFetch."},
				},
				"required":             []string{"url"},
				"additionalProperties": false,
			},
			Permission: Prompt,
			Run:        runWebFetch,
		},
	}
}
