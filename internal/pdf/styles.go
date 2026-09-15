package pdf

const styles = `
@page {
	size: A4;
	margin: 18mm 17mm;
}

* {
	box-sizing: border-box;
}

html {
	font: 11pt/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
	color: #18181b;
}

body {
	margin: 0;
}

main {
	max-width: 100%;
}

.document-title {
	margin: 0 0 20px;
	padding-bottom: 12px;
	border-bottom: 1px solid #d4d4d8;
	font-size: 25pt;
	line-height: 1.15;
}

h1,
h2,
h3,
h4,
h5,
h6 {
	break-after: avoid;
	margin: 1.5em 0 0.55em;
	line-height: 1.25;
}

h1 {
	font-size: 21pt;
}

h2 {
	font-size: 17pt;
}

h3 {
	font-size: 14pt;
}

p,
ul,
ol,
blockquote,
table,
pre,
.callout,
.markdown-tabs,
.markdown-details {
	break-inside: auto;
}

a {
	color: #1d4ed8;
	text-decoration: none;
}

img {
	max-width: 100%;
	height: auto;
	break-inside: avoid;
}

table {
	width: 100%;
	border-collapse: collapse;
	font-size: 9.5pt;
}

th,
td {
	padding: 6px 8px;
	border: 1px solid #d4d4d8;
	text-align: left;
}

blockquote {
	margin-left: 0;
	padding-left: 14px;
	border-left: 3px solid #64748b;
	color: #475569;
}

pre {
	padding: 12px;
	overflow-wrap: anywhere;
	white-space: pre-wrap;
	border: 1px solid #d4d4d8;
	border-radius: 6px;
	background: #f4f4f5;
	font: 9.5pt/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
}

code {
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.callout {
	margin: 14px 0;
	padding: 10px 12px;
	border: 1px solid #d4d4d8;
	border-left: 4px solid #3b82f6;
	border-radius: 6px;
	background: #fafafa;
}

.callout.warning {
	border-left-color: #d97706;
}

.callout.tip,
.callout.success {
	border-left-color: #16a34a;
}

.callout.danger,
.callout.error {
	border-left-color: #dc2626;
}

.markdown-tab-list {
	display: flex;
	gap: 8px;
	margin: 14px 0 8px;
	padding-bottom: 6px;
	border-bottom: 1px solid #d4d4d8;
}

.markdown-tab-list button {
	border: 0;
	background: none;
	font-weight: 700;
}

.markdown-tab-panel-hidden {
	display: block !important;
}

.markdown-tab-panel {
	padding: 4px 0 10px;
}

.markdown-details {
	margin: 14px 0;
	padding: 0 12px 10px;
	border: 1px solid #d4d4d8;
	border-radius: 6px;
}

.markdown-details summary,
.markdown-details-summary {
	padding: 9px 0;
	font-weight: 700;
}

.markdown-details-body {
	display: block !important;
}
`
