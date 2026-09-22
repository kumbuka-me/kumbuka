// Fenced-code language catalog used by the visual editor language picker.

export interface CodeLanguage {
  label: string;
  value: string;
}

// Chroma 2.27.0 lexer definitions used by the bundled syntax-highlighting plugin.
const CHROMA_LANGUAGES = `
abap abnf actionscript actionscript_3 ada agda al alloy ampl angular2
antlr apacheconf apl applescript arangodb_aql arduino armasm arturo atl autohotkey
autoit awk ballerina bash bash_session batchfile beef caddyfile bibtex bicep
blitzbasic bnf bqn brainfuck c# c++ c c3 cap_n_proto cassandra_cql
ceylon cfengine3 cfstatement chaiscript chapel cheetah clojure cmake cobol coffeescript
common_lisp coq core crystal css csv cue cython d dart
dax desktop_entry devicetree diff django_jinja dns docker dtd dylan ebnf
elixir elm emacslisp erb erlang factor fennel fish forth fortran
fortranfixed fsharp gas gdscript gdscript3 gemfile_lock gemtext gettext gherkin gleam
glsl gnuplot go go_template graphql groff groovy handlebars hare haxe
haskell hcl hexdump hlb hlsl holyc html http hy idris
igor ini io iscdhcpd j janet java javascript json jsonata
jsonnet julia jungle kakoune kdl kotlin lateralus lean lighttpd_configuration_file lilypond
llvm lox lua luau makefile mako markdown markless mason materialize_sql_dialect
mathematica matlab mcfunction meson metal microcad minizinc mlir modelica modula-2
mojo monkeyc moonbit moonscript morrowindscript myghty mysql nasm natural ndisasm
newspeak nginx_configuration_file nim nix nsis nu objective-c objectpascal ocaml octave
odin onesenterprise openedge_abl openscad org_mode pacmanconf perl php pig pkgconfig
pl_pgsql plaintext plutus_core pony postgresql_sql_dialect postscript povray powerquery powershell prolog
promela promql properties protocol_buffer prql psl puppet python python_2 qbasic
qml r racket raku rst ragel react reasonml reg rego
rexx rgbasm ring rpgle rpm_spec ruby rust sas sass scala
scdoc scheme scilab scss sed sieve smali smalltalk smarty snbt
snobol solidity sourcepawn spade sparql sql squidconf standard_ml stas stylus
svelte swift systemd systemverilog tablegen tal tasm tcl tcsh templ
termcap terminfo terraform tex thrift toml tradingview transact-sql turing turtle
twig txtpb typescript typoscript typoscriptcssdata typoscripthtmldata typst ucode v v_shell
vala vb_net verilog vhdl vhs viml vue wat wdte webgpu_shading_language
webvtt whiley xml xorg yaml yaml_jinja yang z80_assembly zed zig
`
  .trim()
  .split(/\s+/);

const LABEL_OVERRIDES: Record<string, string> = {
  "": "Plain text",
  "c#": "C#",
  "c++": "C++",
  abap: "ABAP",
  css: "CSS",
  csv: "CSV",
  dns: "DNS",
  glsl: "GLSL",
  graphql: "GraphQL",
  hcl: "HCL",
  hlsl: "HLSL",
  html: "HTML",
  ini: "INI",
  io: "Io",
  javascript: "JavaScript",
  json: "JSON",
  jsonata: "JSONata",
  jsonnet: "Jsonnet",
  llvm: "LLVM",
  lua: "Lua",
  luau: "Luau",
  matlab: "MATLAB",
  mysql: "MySQL",
  nginx_configuration_file: "Nginx configuration",
  "objective-c": "Objective-C",
  ocaml: "OCaml",
  php: "PHP",
  pl_pgsql: "PL/pgSQL",
  postgresql_sql_dialect: "PostgreSQL SQL",
  powershell: "PowerShell",
  promql: "PromQL",
  protocol_buffer: "Protocol Buffers",
  python_2: "Python 2",
  qml: "QML",
  rpm_spec: "RPM spec",
  scss: "SCSS",
  sql: "SQL",
  systemverilog: "SystemVerilog",
  tcl: "Tcl",
  tex: "TeX / LaTeX",
  toml: "TOML",
  "transact-sql": "Transact-SQL",
  typescript: "TypeScript",
  vb_net: "VB.NET",
  vhdl: "VHDL",
  viml: "Vim script",
  wat: "WebAssembly text",
  webgpu_shading_language: "WebGPU shading language",
  xml: "XML",
  yaml: "YAML",
  z80_assembly: "Z80 assembly",
};

const LANGUAGE_ALIASES: Record<string, string> = {
  plain: "",
  plaintext: "",
  text: "",
  coffee: "coffeescript",
  cpp: "c++",
  csharp: "c#",
  dockerfile: "docker",
  golang: "go",
  js: "javascript",
  jsx: "react",
  md: "markdown",
  py: "python",
  rb: "ruby",
  rs: "rust",
  sh: "bash",
  shell: "bash",
  ts: "typescript",
  tsx: "react",
  vim: "viml",
  wgsl: "webgpu_shading_language",
  yml: "yaml",
  zsh: "bash",
};

// languageLabel converts one Chroma identifier into concise picker copy.
function languageLabel(value: string): string {
  const override = LABEL_OVERRIDES[value];
  if (override) return override;

  return value
    .replaceAll("_", " ")
    .replaceAll("-", " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

// canonicalCodeLanguage resolves common fence aliases to a listed Chroma language.
export function canonicalCodeLanguage(value: string): string {
  const normalized = value.trim().toLowerCase();
  return LANGUAGE_ALIASES[normalized] ?? normalized;
}

// supportedCodeLanguages returns the selectable fenced-code languages.
export function supportedCodeLanguages(): CodeLanguage[] {
  return [
    { label: "Plain text", value: "" },
    ...CHROMA_LANGUAGES.filter((value) => value !== "plaintext").map(
      (value) => ({ label: languageLabel(value), value }),
    ),
  ];
}

// codeLanguageLabel returns readable copy for a stored fence language.
export function codeLanguageLabel(value: string): string {
  const normalized = value.trim().toLowerCase();
  if (!normalized) return "Plain text";
  const canonical = canonicalCodeLanguage(normalized);
  if (!canonical || canonical === "plaintext") return "Plain text";
  return CHROMA_LANGUAGES.includes(canonical)
    ? languageLabel(canonical)
    : value;
}

// filterCodeLanguages filters supported languages by label, identifier, and common aliases.
export function filterCodeLanguages(query: string): CodeLanguage[] {
  const normalized = query.trim().toLowerCase();
  const languages = supportedCodeLanguages();
  if (!normalized) return languages;

  return languages.filter((language) => {
    if (
      language.label.toLowerCase().includes(normalized) ||
      language.value.toLowerCase().includes(normalized)
    )
      return true;

    return Object.entries(LANGUAGE_ALIASES).some(
      ([alias, canonical]) =>
        canonical === language.value && alias.includes(normalized),
    );
  });
}
