package mailer

import (
	"html/template"
	"strings"
)

// ManualBodyHTML 将管理员手写的纯文本正文转义为安全的 HTML 片段（换行转 <br>）。
func ManualBodyHTML(text string) template.HTML {
	escaped := template.HTMLEscapeString(strings.TrimSpace(text))
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br />")
	return template.HTML(escaped)
}
