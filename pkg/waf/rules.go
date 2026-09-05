package waf

import "regexp"

func defaultRules() []WAFRule {
	return []WAFRule{
		// --- SQL Injection (SQLi) Rules ---
		{
			ID:          "SQLI-001",
			Category:    CategorySQLi,
			Description: "Detects SQL UNION SELECT injection pattern",
			Pattern:     regexp.MustCompile(`(?i)\bunion\s+(all\s+)?select\b`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "SQLI-002",
			Category:    CategorySQLi,
			Description: "Detects SQL inline boolean evaluation pattern (' OR 1=1)",
			Pattern:     regexp.MustCompile(`(?i)('\s*or\s*'?1'?\s*=\s*'?1'?|"\s*or\s*"?1"?\s*=\s*"?1"?|\bor\s+true\b)`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "SQLI-003",
			Category:    CategorySQLi,
			Description: "Detects SQL DDL/DML destructive statements (DROP TABLE, INSERT INTO)",
			Pattern:     regexp.MustCompile(`(?i)\b(drop\s+table|insert\s+into|delete\s+from|alter\s+table|exec(\s|\+)+(sp_|xp_))\b`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},

		// --- Cross-Site Scripting (XSS) Rules ---
		{
			ID:          "XSS-001",
			Category:    CategoryXSS,
			Description: "Detects HTML script tag injection (<script>)",
			Pattern:     regexp.MustCompile(`(?i)<script\b[^>]*>`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "XSS-002",
			Category:    CategoryXSS,
			Description: "Detects inline JavaScript event handler attributes (onerror=, onload=)",
			Pattern:     regexp.MustCompile(`(?i)\b(on(load|error|click|mouseover|submit|focus|blur))\s*=`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "XSS-003",
			Category:    CategoryXSS,
			Description: "Detects javascript: protocol scheme injection",
			Pattern:     regexp.MustCompile(`(?i)javascript\s*:`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},

		// --- Path Traversal / LFI Rules ---
		{
			ID:          "TRAVERSAL-001",
			Category:    CategoryTraversal,
			Description: "Detects relative directory traversal sequence (../ or ..\\)",
			Pattern:     regexp.MustCompile(`(?i)(\.\./|\.\.\\|%2e%2e/|%2e%2e%2f|%2e%2e\\|%2e%2e%5c)`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "TRAVERSAL-002",
			Category:    CategoryTraversal,
			Description: "Detects sensitive system file path access (/etc/passwd, /win.ini)",
			Pattern:     regexp.MustCompile(`(?i)(/etc/passwd|/etc/shadow|c:\\windows\\system32|win\.ini)`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},

		// --- Command Injection / RCE Rules ---
		{
			ID:          "RCE-001",
			Category:    CategoryRCE,
			Description: "Detects shell command chaining and backtick execution (; /bin/sh, | bash)",
			Pattern:     regexp.MustCompile(`(?i)(;\s*(/bin/sh|/bin/bash|cmd\.exe|powershell)|\|\s*(bash|sh)|` + "`" + `[^` + "`" + `]+` + "`" + `)`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
		{
			ID:          "RCE-002",
			Category:    CategoryRCE,
			Description: "Detects PHP/system code execution primitives (eval(), system(), passthru())",
			Pattern:     regexp.MustCompile(`(?i)\b(eval\(|passthru\(|shell_exec\(|system\()\b`),
			Score:       5,
			Locations:   InspectURL | InspectQuery | InspectHeaders | InspectBody,
		},
	}
}
