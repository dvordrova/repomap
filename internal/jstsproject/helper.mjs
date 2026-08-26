import path from "node:path"
import process from "node:process"
import { createRequire } from "node:module"
import { existsSync, readFileSync } from "node:fs"
import { createHash } from "node:crypto"
import { pathToFileURL } from "node:url"

const CONTRACT_VERSION = 4
const MAX_INPUT_BYTES = 32 * 1024 * 1024
const MAX_CONFIG_BYTES = 1024 * 1024
const MAX_PROJECT_CONFIGS = 32
const MAX_PROJECT_REFERENCE_DEPTH = 16
const MAX_STABLE_ID_PART_CHARS = 240
const MAX_EXPRESSION_TEXT_CHARS = 512
const MAX_SIGNATURE_TEXT_CHARS = 2048
const MAX_NPM_SCOPED_PACKAGE_PARTS = 2
const MAX_EXTERNAL_PART_CHARS = 256

function fail(message) {
  process.stderr.write(`jsts helper: ${message}\n`)
  process.exit(1)
}

let raw = ""
for await (const chunk of process.stdin) {
  raw += chunk
  if (Buffer.byteLength(raw) > MAX_INPUT_BYTES) fail("input exceeds limit")
}

let request
try {
  request = JSON.parse(raw)
} catch {
  fail("invalid JSON input")
}
if (request.version !== CONTRACT_VERSION || !Array.isArray(request.files) || !Array.isArray(request.compiler_packages)) {
  fail("unsupported input contract")
}

const compareText = (left, right) => left < right ? -1 : left > right ? 1 : 0
const slash = (value) => value.split(path.sep).join("/")
const cleanRelative = (value) => {
  if (typeof value !== "string" || value.length === 0 || value.includes("\\")) return ""
  const normalized = path.posix.normalize(value)
  if (normalized !== value || normalized === "." || normalized.startsWith("../") || path.posix.isAbsolute(normalized)) return ""
  return normalized
}
const repositoryRoot = path.resolve(process.cwd())
const requestedProjectDir = request.project_dir === undefined || request.project_dir === "" ? "" : cleanRelative(request.project_dir)
if (request.project_dir !== undefined && request.project_dir !== "" && !requestedProjectDir) {
  fail("invalid project directory")
}
const root = requestedProjectDir ? path.join(repositoryRoot, ...requestedProjectDir.split("/")) : repositoryRoot
const rootPrefix = root.endsWith(path.sep) ? root : `${root}${path.sep}`
const absolute = (relative) => path.join(root, ...relative.split("/"))
const relative = (filename) => {
  const resolved = path.resolve(filename)
  if (resolved !== root && !resolved.startsWith(rootPrefix)) return ""
  return cleanRelative(slash(path.relative(root, resolved)))
}

const requestedCompilerPackages = []
const validPackageName = /^(?:@[a-z0-9][a-z0-9._~-]*\/)?[a-z0-9][a-z0-9._~-]*$/
const compilerResolutionBases = new Set(["project", "repository_root"])
for (const candidate of request.compiler_packages) {
  const packageName = candidate?.name
  const resolutionBase = candidate?.resolution_base
  const candidateKey = `${resolutionBase}\0${packageName}`
  if (typeof candidate !== "object" || candidate === null || Array.isArray(candidate) ||
      typeof packageName !== "string" || !validPackageName.test(packageName) ||
      typeof resolutionBase !== "string" || !compilerResolutionBases.has(resolutionBase) ||
      Object.keys(candidate).some((key) => key !== "name" && key !== "resolution_base") ||
      (requestedCompilerPackages.length > 0 &&
        (requestedCompilerPackages.at(-1).resolutionBase !== resolutionBase ||
          compareText(requestedCompilerPackages.at(-1).key, candidateKey) >= 0))) {
    fail("invalid TypeScript compiler package candidates")
  }
  requestedCompilerPackages.push({ packageName, resolutionBase, key: candidateKey })
}
if (requestedCompilerPackages.length === 0) fail("no manifest-declared TypeScript compiler package candidate")

const fileRefByPath = new Map()
for (const file of request.files) {
  const filePath = cleanRelative(file?.path)
  if (!filePath || typeof file?.file_ref !== "string" || file.file_ref.length === 0 || fileRefByPath.has(filePath)) {
    fail("invalid source-file input")
  }
  fileRefByPath.set(filePath, file.file_ref)
}

let ts
let compilerFlavor = "legacy"
let nativeAPI
let nativeSnapshot
try {
  const projectRequire = createRequire(path.join(root, "package.json"))
  const repositoryRootRequire = createRequire(path.join(repositoryRoot, "package.json"))
  const allowedCompilerRoots = [...new Set([
    path.join(root, "node_modules"),
    path.join(repositoryRoot, "node_modules"),
  ])]
  const candidatesByPath = new Map()
  const rejected = []
  for (const requestedCompiler of requestedCompilerPackages) {
    const { packageName, resolutionBase } = requestedCompiler
    const compilerRequire = resolutionBase === "repository_root" ? repositoryRootRequire : projectRequire
    let packagePath
    try {
      packagePath = path.resolve(compilerRequire.resolve(`${packageName}/package.json`))
    } catch {
      rejected.push(`${packageName}: package is not installed`)
      continue
    }
    if (!allowedCompilerRoots.some((nodeModulesRoot) => packagePath.startsWith(`${nodeModulesRoot}${path.sep}`))) {
      rejected.push(`${packageName}: package is outside analyzed node_modules`)
      continue
    }
    const packageRoot = path.dirname(packagePath)
    let packageDocument
    try {
      packageDocument = JSON.parse(readFileSync(packagePath, "utf8"))
    } catch {
      rejected.push(`${packageName}: package.json is invalid`)
      continue
    }
    if (packageDocument?.name !== "typescript") {
      rejected.push(`${packageName}: installed package name is not typescript`)
      continue
    }
    const exportedFile = (key) => {
      const target = packageDocument?.exports?.[key]
      if (typeof target !== "string" || !target.startsWith("./")) return ""
      const resolved = path.resolve(packageRoot, target)
      const prefix = packageRoot.endsWith(path.sep) ? packageRoot : `${packageRoot}${path.sep}`
      return resolved.startsWith(prefix) && existsSync(resolved) ? resolved : ""
    }
    const legacyPath = path.join(packageRoot, "lib", "typescript.js")
    const legacy = existsSync(legacyPath)
    const syncPath = legacy ? "" : exportedFile("./unstable/sync")
    const astPath = legacy ? "" : exportedFile("./unstable/ast")
    if (!legacy && (!syncPath || !astPath)) {
      rejected.push(`${packageName}: package exposes no supported Compiler API`)
      continue
    }
    if (!candidatesByPath.has(packagePath)) {
      candidatesByPath.set(packagePath, {
        packageName, packagePath, packageRoot, packageDocument,
        flavor: legacy ? "legacy" : "native", legacyPath, syncPath, astPath, compilerRequire,
      })
    }
  }
  const candidates = [...candidatesByPath.values()].sort((left, right) => compareText(left.packageName, right.packageName))
  const legacyCandidates = candidates.filter((candidate) => candidate.flavor === "legacy")
  const preferred = legacyCandidates.length > 0 ? legacyCandidates : candidates
  if (preferred.length === 0) {
    const detail = rejected.length > 0 ? `: ${rejected.join("; ")}` : ""
    throw new Error(`no supported manifest-declared TypeScript compiler is prepared${detail}`)
  }
  if (preferred.length > 1) {
    throw new Error(`ambiguous supported TypeScript compiler packages: ${preferred.map((candidate) => candidate.packageName).join(", ")}`)
  }
  const selectedCompiler = preferred[0]
  if (selectedCompiler.flavor === "legacy") {
    // Resolve the package root first, then load the compiler-owned file. A
    // package may legitimately export only its public root while retaining
    // the backwards-compatible Compiler API file on disk; asking Node to
    // resolve the private subpath would incorrectly reject that package.
    ts = selectedCompiler.compilerRequire(selectedCompiler.legacyPath)
  } else {
    const [syncAPI, astAPI] = await Promise.all([
      import(pathToFileURL(selectedCompiler.syncPath).href),
      import(pathToFileURL(selectedCompiler.astPath).href),
    ])
    if (typeof syncAPI.API !== "function" || !astAPI.SyntaxKind) throw new Error("prepared TypeScript native API is incomplete")
    compilerFlavor = "native"
    ts = {
      ...astAPI,
      ...syncAPI,
      forEachChild: (node, visitor) => node.forEachChild(visitor),
      isFunctionLike: (node) => astAPI.isFunctionDeclaration(node) || astAPI.isMethodDeclaration(node) ||
        astAPI.isConstructorDeclaration(node) || astAPI.isGetAccessorDeclaration(node) ||
        astAPI.isSetAccessorDeclaration(node) || astAPI.isFunctionExpression(node) ||
        astAPI.isArrowFunction(node),
    }
    nativeAPI = new syncAPI.API({ cwd: root })
  }
} catch (error) {
  fail(`load prepared TypeScript compiler: ${error instanceof Error ? error.message : "unknown error"}`)
}

let configPath = ""
let moduleResolution = "nodenext"
let baseURL = ""
let pathAliases = []

function parseJSONC(content) {
  let clean = ""
  let inString = false
  let escaped = false
  let lineComment = false
  let blockComment = false
  for (let index = 0; index < content.length; index++) {
    const value = content[index]
    const next = content[index + 1]
    if (lineComment) {
      if (value === "\n" || value === "\r") {
        lineComment = false
        clean += value
      } else clean += " "
      continue
    }
    if (blockComment) {
      if (value === "*" && next === "/") {
        blockComment = false
        clean += "  "
        index++
      } else clean += value === "\n" || value === "\r" ? value : " "
      continue
    }
    if (inString) {
      clean += value
      if (escaped) escaped = false
      else if (value === "\\") escaped = true
      else if (value === '"') inString = false
      continue
    }
    if (value === '"') {
      inString = true
      clean += value
    } else if (value === "/" && next === "/") {
      lineComment = true
      clean += "  "
      index++
    } else if (value === "/" && next === "*") {
      blockComment = true
      clean += "  "
      index++
    } else clean += value
  }
  if (inString || blockComment) throw new Error("unterminated JSONC token")
  let withoutTrailingCommas = ""
  inString = false
  escaped = false
  for (let index = 0; index < clean.length; index++) {
    const value = clean[index]
    if (inString) {
      withoutTrailingCommas += value
      if (escaped) escaped = false
      else if (value === "\\") escaped = true
      else if (value === '"') inString = false
      continue
    }
    if (value === '"') {
      inString = true
      withoutTrailingCommas += value
      continue
    }
    if (value === ",") {
      let next = index + 1
      while (next < clean.length && /\s/.test(clean[next])) next++
      if (clean[next] === "}" || clean[next] === "]") continue
    }
    withoutTrailingCommas += value
  }
  return JSON.parse(withoutTrailingCommas)
}

function rawConfig(configFile) {
  const bytes = readFileSync(absolute(configFile))
  if (bytes.length > MAX_CONFIG_BYTES) fail(`project config exceeds ${MAX_CONFIG_BYTES} bytes`)
  try {
    return parseJSONC(bytes.toString("utf8"))
  } catch (error) {
    fail(`parse project config ${configFile}: ${error instanceof Error ? error.message : "invalid JSONC"}`)
  }
}

function referencedConfig(configFile, referencePath) {
  if (typeof referencePath !== "string" || referencePath.length === 0 || referencePath.includes("\\")) {
    fail(`invalid project reference in ${configFile}`)
  }
  const base = path.resolve(path.dirname(absolute(configFile)), referencePath)
  const candidates = path.extname(base) ? [base] : [path.join(base, "tsconfig.json"), `${base}.json`]
  for (const candidate of candidates) {
    const config = relative(candidate)
    if (config && existsSync(candidate)) return config
  }
  fail(`unresolved project reference ${referencePath} in ${configFile}`)
}

function configGraph(rootConfig) {
  const visited = new Set()
  const active = new Set()
  const result = []
  const visit = (configFile, depth) => {
    if (depth > MAX_PROJECT_REFERENCE_DEPTH) fail("project reference depth exceeds limit")
    if (active.has(configFile)) fail(`project reference cycle at ${configFile}`)
    if (visited.has(configFile)) return
    if (visited.size >= MAX_PROJECT_CONFIGS) fail("project config count exceeds limit")
    visited.add(configFile)
    active.add(configFile)
    const document = rawConfig(configFile)
    result.push({ path: configFile, document })
    const references = document?.references
    if (references !== undefined && !Array.isArray(references)) fail(`invalid project references in ${configFile}`)
    const children = (references || []).map((value) => referencedConfig(configFile, value?.path)).sort(compareText)
    for (const child of children) visit(child, depth + 1)
    active.delete(configFile)
  }
  visit(rootConfig, 0)
  return result.sort((left, right) => compareText(left.path, right.path))
}

const additionalFiles = []
if (Array.isArray(request.additional_files)) {
  for (const candidate of request.additional_files) {
    const filePath = cleanRelative(candidate)
    if (!filePath || !fileRefByPath.has(filePath)) fail("invalid additional source file")
    additionalFiles.push(filePath)
  }
}
additionalFiles.sort(compareText)

let configRecords = []
if (typeof request.config_path === "string" && request.config_path.length > 0) {
  configPath = cleanRelative(request.config_path)
  if (!configPath) fail("invalid config path")
  configRecords = configGraph(configPath)
}

const compilerProjects = []
if (compilerFlavor === "legacy") {
  const defaults = {
    allowJs: true,
    checkJs: false,
    noEmit: true,
    moduleResolution: ts.ModuleResolutionKind.NodeNext,
    module: ts.ModuleKind.NodeNext,
    jsx: ts.JsxEmit.Preserve,
  }
  if (configRecords.length === 0) {
    const options = defaults
    const program = ts.createProgram({ rootNames: [...fileRefByPath.keys()].sort(compareText).map(absolute), options })
    compilerProjects.push({ key: "", options, program, checker: program.getTypeChecker(), rootFiles: new Set(fileRefByPath.keys()) })
  } else {
    for (const record of configRecords) {
      const read = ts.readConfigFile(absolute(record.path), ts.sys.readFile)
      if (read.error) fail(ts.flattenDiagnosticMessageText(read.error.messageText, " "))
      const parsed = ts.parseJsonConfigFileContent(read.config, ts.sys, path.dirname(absolute(record.path)), undefined, absolute(record.path))
      if (parsed.errors.length > 0) fail(ts.flattenDiagnosticMessageText(parsed.errors[0].messageText, " "))
      const rootFiles = new Set(parsed.fileNames.map(relative).filter((value) => fileRefByPath.has(value)))
      if (rootFiles.size === 0) continue
      const options = { ...parsed.options, noEmit: true }
      const program = ts.createProgram({ rootNames: [...rootFiles].sort(compareText).map(absolute), options })
      compilerProjects.push({ key: record.path, options, program, checker: program.getTypeChecker(), rootFiles })
    }
    const additionalRoots = additionalFiles.filter((filePath) => !compilerProjects.some((project) => project.rootFiles.has(filePath)))
    if (additionalRoots.length > 0 && compilerProjects.length > 0) {
      const options = compilerProjects[0].options
      const program = ts.createProgram({ rootNames: additionalRoots.map(absolute), options })
      compilerProjects.push({ key: "~additional-files", options, program, checker: program.getTypeChecker(), rootFiles: new Set(additionalRoots) })
    }
  }
} else {
  let openProjects = []
  if (configRecords.length > 0) {
    for (const record of configRecords) {
      const parsed = nativeAPI.parseConfigFile(absolute(record.path))
      const rootFiles = new Set(parsed.fileNames.map(relative).filter((value) => fileRefByPath.has(value)))
      if (rootFiles.size > 0) openProjects.push(record.path)
    }
  }
  openProjects = [...new Set(openProjects)].sort(compareText)
  const openFiles = configRecords.length === 0 ? [...fileRefByPath.keys()].sort(compareText) : additionalFiles
  nativeSnapshot = nativeAPI.updateSnapshot({
    openProjects: openProjects.map(absolute),
    openFiles: openFiles.map(absolute),
  })
  for (const configFile of openProjects) {
    const project = nativeSnapshot.getProject(absolute(configFile))
    if (!project) fail(`native Compiler API omitted project ${configFile}`)
    compilerProjects.push({
      key: configFile,
      options: project.compilerOptions,
      program: project.program,
      checker: project.checker,
      rootFiles: new Set(project.rootFiles.map(relative).filter((value) => fileRefByPath.has(value))),
    })
  }
  for (const filePath of openFiles) {
    const project = nativeSnapshot.getDefaultProjectForFile(absolute(filePath))
    if (!project || compilerProjects.some((value) => value.program === project.program)) continue
    compilerProjects.push({
      key: relative(project.configFileName) || `~inferred:${filePath}`,
      options: project.compilerOptions,
      program: project.program,
      checker: project.checker,
      rootFiles: new Set(project.rootFiles.map(relative).filter((value) => fileRefByPath.has(value))),
    })
  }
}

compilerProjects.sort((left, right) => compareText(left.key, right.key))
if (compilerProjects.length === 0) fail("configuration selects no tracked JavaScript or TypeScript files")

const resolutionNames = new Map([[1, "classic"], [2, "node10"], [3, "node16"], [99, "nodenext"], [100, "bundler"]])
const resolutionValues = [...new Set(compilerProjects.map((value) => resolutionNames.get(value.options.moduleResolution) || "unspecified"))].sort(compareText)
moduleResolution = resolutionValues.length === 1 ? resolutionValues[0] : "mixed"
const baseURLs = [...new Set(compilerProjects.map((value) => value.options.baseUrl ? relative(value.options.baseUrl) || "." : "").filter(Boolean))].sort(compareText)
baseURL = baseURLs.length === 1 ? baseURLs[0] : ""
const aliases = new Map()
for (const project of compilerProjects) {
  const aliasBase = project.options.pathsBasePath || (project.key ? path.dirname(absolute(project.key)) : root)
  for (const [pattern, targets] of Object.entries(project.options.paths || {})) {
    const values = aliases.get(pattern) || new Set()
    for (const target of targets || []) {
      const marker = "__REPOMAP_ALIAS_STAR__"
      const normalized = relative(path.resolve(aliasBase, String(target).replaceAll("*", marker))).replaceAll(marker, "*")
      if (normalized) values.add(normalized)
    }
    aliases.set(pattern, values)
  }
}
pathAliases = [...aliases.entries()]
  .map(([pattern, targets]) => ({ pattern, targets: [...targets].sort(compareText) }))
  .sort((left, right) => compareText(left.pattern, right.pattern))

const sourceByPath = new Map()
for (const project of compilerProjects) {
  const names = compilerFlavor === "legacy" ? project.program.getSourceFiles().map((value) => value.fileName) : project.program.getSourceFileNames()
  for (const filename of names) {
    const filePath = relative(filename)
    if (!fileRefByPath.has(filePath) || sourceByPath.has(filePath)) continue
    const sourceFile = compilerFlavor === "legacy" ? project.program.getSourceFile(filename) : project.program.getSourceFile(filename)
    if (sourceFile) sourceByPath.set(filePath, { sourceFile, path: filePath, project })
  }
}
for (const filePath of additionalFiles) {
  if (sourceByPath.has(filePath)) continue
  for (const project of compilerProjects) {
    const sourceFile = project.program.getSourceFile(absolute(filePath))
    if (sourceFile) {
      sourceByPath.set(filePath, { sourceFile, path: filePath, project })
      break
    }
  }
}
const sourceFiles = [...sourceByPath.values()].sort((left, right) => compareText(left.path, right.path))
if (sourceFiles.length === 0) fail("configuration selects no tracked JavaScript or TypeScript files")
const projectForNode = (node) => sourceByPath.get(relative(node.getSourceFile().fileName))?.project
const checkerForNode = (node) => projectForNode(node)?.checker
const optionsForNode = (node) => projectForNode(node)?.options || {}
const sourceByteSHA = new Map(sourceFiles.map(({ path: filePath }) => [
  filePath,
  createHash("sha256").update(readFileSync(absolute(filePath))).digest("hex"),
]))

const files = sourceFiles.map(({ path: filePath }) => ({
  file_ref: fileRefByPath.get(filePath),
  path: filePath,
  language: /\.(?:ts|tsx)$/.test(filePath) ? "typescript" : "javascript",
  module: filePath.replace(/\.(?:d\.)?[cm]?[jt]sx?$/, ""),
  sha256: sourceByteSHA.get(filePath) || "",
}))

const locationOf = (node) => {
  const sourceFile = node.getSourceFile()
  const filePath = relative(sourceFile.fileName)
  const point = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile, false))
  return { path: filePath, file_ref: fileRefByPath.get(filePath), line: point.line + 1, column: point.character + 1 }
}
const stablePart = (value) => String(value).replace(/[^A-Za-z0-9_.:/@#-]+/g, "_").slice(0, MAX_STABLE_ID_PART_CHARS)
const factRef = (prefix, node, detail = "") => {
  const loc = locationOf(node)
  return `${prefix}:${loc.file_ref}:${loc.line}:${loc.column}:${stablePart(detail)}`
}
const expressionText = (node) => node.getText(node.getSourceFile()).replace(/\s+/g, " ").trim().slice(0, MAX_EXPRESSION_TEXT_CHARS).trimEnd()
const moduleRef = (filePath) => `module:${fileRefByPath.get(filePath)}`
const moduleName = (filePath) => filePath.replace(/\.(?:d\.)?[cm]?[jt]sx?$/, "")

const declarations = []
const declarationRefByNode = new Map()
const declarationNodeByRef = new Map()

for (const { sourceFile, path: filePath } of sourceFiles) {
  const loc = { path: filePath, file_ref: fileRefByPath.get(filePath), line: 1, column: 1 }
  const ref = moduleRef(filePath)
  declarations.push({ ref, kind: "module", name: moduleName(filePath), qualified_name: moduleName(filePath), exported: false, location: loc })
  declarationNodeByRef.set(ref, sourceFile)
}

function declarationName(node) {
  if (node.name && ts.isIdentifier(node.name)) return node.name.text
  if (ts.isConstructorDeclaration(node)) return "constructor"
  if (node.name && (ts.isStringLiteral(node.name) || ts.isNumericLiteral(node.name))) return node.name.text
  if ((ts.isArrowFunction(node) || ts.isFunctionExpression(node)) && ts.isReturnStatement(node.parent)) return "returned_handler"
  return ""
}

function declarationKind(node) {
  if (ts.isFunctionDeclaration(node)) return "function"
  if (ts.isMethodDeclaration(node) || ts.isConstructorDeclaration(node) || ts.isGetAccessorDeclaration(node) || ts.isSetAccessorDeclaration(node)) return "method"
  if (ts.isClassDeclaration(node) || ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node) || ts.isEnumDeclaration(node)) return "type"
  if (ts.isVariableDeclaration(node)) {
    return node.initializer && (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer)) ? "function" : "variable"
  }
  if ((ts.isArrowFunction(node) || ts.isFunctionExpression(node)) && ts.isReturnStatement(node.parent)) return "lambda"
  return ""
}

function declarationExported(node) {
  let current = ts.isVariableDeclaration(node) ? node.parent?.parent : node
  return Boolean(current?.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword || modifier.kind === ts.SyntaxKind.DefaultKeyword))
}

function namedParent(node) {
  let current = node.parent
  while (current && !ts.isSourceFile(current)) {
    if (declarationRefByNode.has(current)) return current
    current = current.parent
  }
  return undefined
}

function signatureOf(node) {
  const safeTypeText = (value) => {
    value = value.split(slash(rootPrefix)).join("")
    value = value.replace(/node_modules\/(?:@types\/)?(@[^/]+\/[^/]+|[^/]+)(?:\/[^"']*)?/g, "$1")
    return value.slice(0, MAX_SIGNATURE_TEXT_CHARS)
  }
  try {
    const checker = checkerForNode(node)
    if (!checker) return ""
    if (ts.isFunctionLike(node) && typeof checker.signatureToString === "function") {
      const signature = checker.getSignatureFromDeclaration(node)
      return signature ? safeTypeText(checker.signatureToString(signature, node, ts.TypeFormatFlags?.NoTruncation || 0)) : ""
    }
    if (node.name) return safeTypeText(checker.typeToString(checker.getTypeAtLocation(node.name), node, ts.TypeFormatFlags?.NoTruncation || 0))
  } catch {}
  return ""
}

function collectDeclarationNodes(sourceFile) {
  const found = []
  const visit = (node) => {
    const kind = declarationKind(node)
    const name = declarationName(node)
    if (kind && name) found.push({ node, kind, name })
    ts.forEachChild(node, visit)
  }
  visit(sourceFile)
  return found
}

for (const { sourceFile, path: filePath } of sourceFiles) {
  for (const { node, kind, name } of collectDeclarationNodes(sourceFile)) {
    const loc = locationOf(node.name || node)
    const ref = `decl:${loc.file_ref}:${loc.line}:${loc.column}:${stablePart(kind)}:${stablePart(name)}`
    declarationRefByNode.set(node, ref)
    declarationNodeByRef.set(ref, node)
  }
  const visit = (node) => {
    if (declarationRefByNode.has(node)) {
      const kind = declarationKind(node)
      const name = declarationName(node)
      const ownerNode = namedParent(node)
      const ownerRef = ownerNode ? declarationRefByNode.get(ownerNode) : ""
      const ownerName = ownerNode ? declarationName(ownerNode) : ""
      declarations.push({
        ref: declarationRefByNode.get(node),
        kind,
        name,
        qualified_name: `${moduleName(filePath)}#${ownerName ? `${ownerName}.` : ""}${name}`,
        signature: signatureOf(node),
        exported: declarationExported(node),
        owner_ref: ownerRef,
        location: locationOf(node.name || node),
      })
    }
    ts.forEachChild(node, visit)
  }
  visit(sourceFile)
}

const refForDeclarationNode = (node) => {
  let current = node
  while (current && !ts.isSourceFile(current)) {
    if (declarationRefByNode.has(current)) return declarationRefByNode.get(current)
    if (ts.isVariableDeclaration(current) && declarationRefByNode.has(current)) return declarationRefByNode.get(current)
    current = current.parent
  }
  const sourceFile = node.getSourceFile()
  return moduleRef(relative(sourceFile.fileName))
}

function symbolDeclarations(symbol) {
  const result = []
  for (const declaration of symbol?.declarations || []) {
    const node = typeof declaration?.resolve === "function" ? declaration.resolve() : declaration
    if (node) result.push(node)
  }
  return result
}

function refsForSymbol(symbol, checker) {
  if (!symbol) return []
  try {
    if (symbol.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol)
  } catch {}
  const refs = []
  for (const declaration of symbolDeclarations(symbol)) {
    let current = declaration
    while (current && !ts.isSourceFile(current) && !declarationRefByNode.has(current)) current = current.parent
    if (declarationRefByNode.has(current)) refs.push(declarationRefByNode.get(current))
  }
  return [...new Set(refs)].sort()
}

function refsForExpression(node) {
  const checker = checkerForNode(node)
  if (!checker) return []
  let symbol
  try { symbol = checker.getSymbolAtLocation(node) } catch {}
  let refs = refsForSymbol(symbol, checker)
  if (refs.length === 0 && ts.isCallExpression(node)) refs = refsForExpression(node.expression)
  if (refs.length === 0 && ts.isPropertyAccessExpression(node)) refs = refsForExpression(node.name)
  return refs
}

function resolvedSignatureDeclaration(node, checker) {
  try {
    const signature = checker.getResolvedSignature(node)
    const declaration = signature?.declaration || (typeof signature?.getDeclaration === "function" ? signature.getDeclaration() : undefined)
    return typeof declaration?.resolve === "function" ? declaration.resolve() : declaration
  } catch {
    return undefined
  }
}

function localRefsForInvocation(node) {
  const checker = checkerForNode(node)
  if (!checker) return []
  const declaration = resolvedSignatureDeclaration(node, checker)
  let current = declaration
  while (current && !ts.isSourceFile(current) && !declarationRefByNode.has(current)) current = current.parent
  return declarationRefByNode.has(current) ? [declarationRefByNode.get(current)] : []
}

function resolvedCallableSymbol(node, checker) {
  let location = node.expression
  if (ts.isPropertyAccessExpression(location)) location = location.name
  else if (typeof ts.isElementAccessExpression === "function" && ts.isElementAccessExpression(location)) location = location.argumentExpression
  let symbol
  try { symbol = checker.getSymbolAtLocation(location) } catch {}
  try {
    if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol)
  } catch {}
  return symbol
}

function canonicalDeclarationName(node) {
  const name = node?.name
  if (name && (ts.isIdentifier(name) || ts.isStringLiteral(name) || ts.isNumericLiteral(name))) return String(name.text)
  return ""
}

function namedDeclarationAtOrAbove(node) {
  let current = node
  while (current && !ts.isSourceFile(current)) {
    const name = canonicalDeclarationName(current)
    if (name) return { node: current, name }
    current = current.parent
  }
  return undefined
}

function safeExternalPart(value) {
  if (typeof value !== "string" || value.length === 0 || value.length > MAX_EXTERNAL_PART_CHARS || value.trim() !== value || /[\0\r\n]/.test(value)) return ""
  return value
}

function canonicalPlatformTarget(declaration) {
  const named = namedDeclarationAtOrAbove(declaration)
  if (!named) return undefined
  const owner = namedDeclarationAtOrAbove(named.node.parent)
  const name = safeExternalPart(named.name)
  const receiver = owner ? safeExternalPart(owner.name) : ""
  if (!name || (owner && !receiver)) return undefined
  return { receiver, name }
}

function platformTargetForInvocation(node) {
  const project = projectForNode(node)
  const checker = project?.checker
  const program = project?.program
  if (!checker || typeof program?.isSourceFileDefaultLibrary !== "function") return undefined
  const symbol = resolvedCallableSymbol(node, checker)
  // Name the boundary from the symbol that the source actually invokes. A
  // constructor's resolved signature may live on a differently named helper
  // interface (Date -> DateConstructor, Promise -> PromiseConstructor); that
  // type-level implementation detail is not the invoked runtime identity.
  // Requiring default-library authority on the invoked symbol itself also
  // prevents a repository-local value merely typed as DateConstructor from
  // being promoted to the JavaScript platform.
  const distinctDeclarations = [...new Set(symbolDeclarations(symbol))]
  if (distinctDeclarations.length === 0) return undefined
  let hasDefaultLibraryAuthority = false
  const targets = new Map()
  for (const declaration of distinctDeclarations) {
    let defaultLibrary = false
    try { defaultLibrary = program.isSourceFileDefaultLibrary(declaration.getSourceFile()) } catch {}
    hasDefaultLibraryAuthority ||= defaultLibrary
    const target = canonicalPlatformTarget(declaration)
    if (!target) return undefined
    targets.set(`${target.receiver}\0${target.name}`, target)
  }
  if (!hasDefaultLibraryAuthority || targets.size !== 1) return undefined
  return targets.values().next().value
}

const imports = []
const exports = []
const importAuthorityBySymbol = new Map()
const packageRoot = (specifier) => specifier.startsWith("@") ? specifier.split("/").slice(0, MAX_NPM_SCOPED_PACKAGE_PARTS).join("/") : specifier.split("/")[0]
const isExternalSpecifier = (specifier) => !specifier.startsWith(".") && !specifier.startsWith("/") && !request.path_alias_prefixes?.some((prefix) => specifier.startsWith(prefix))

function resolutionFor(specifier, containingFile, specifierNode) {
  let resolvedFileName = ""
  if (compilerFlavor === "legacy") {
    const resolved = ts.resolveModuleName(specifier, containingFile, optionsForNode(specifierNode), ts.sys).resolvedModule
    resolvedFileName = resolved?.resolvedFileName || ""
  } else {
    const checker = checkerForNode(specifierNode)
    let symbol
    try { symbol = checker?.getSymbolAtLocation(specifierNode) } catch {}
    for (const declaration of symbolDeclarations(symbol)) {
      const candidate = declaration.getSourceFile()?.fileName || ""
      if (candidate) {
        resolvedFileName = candidate
        break
      }
    }
  }
  const resolved = resolvedFileName !== ""
  if (resolved) {
    const targetPath = relative(resolvedFileName)
    if (targetPath && fileRefByPath.has(targetPath)) return { resolution: "exact", resolved_file_ref: fileRefByPath.get(targetPath), external_package: "" }
    if (isExternalSpecifier(specifier)) return { resolution: "exact", resolved_file_ref: "", external_package: packageRoot(specifier) }
  }
  if (isExternalSpecifier(specifier)) return { resolution: "unresolved", resolved_file_ref: "", external_package: packageRoot(specifier) }
  return { resolution: "unresolved", resolved_file_ref: "", external_package: "" }
}

function bindImportSymbols(node, externalPackage, resolution) {
  if (!externalPackage || !node.importClause) return
  const checker = checkerForNode(node)
  if (!checker) return
  const names = []
  if (node.importClause.name) names.push(node.importClause.name)
  const bindings = node.importClause.namedBindings
  if (bindings && ts.isNamespaceImport(bindings)) names.push(bindings.name)
  if (bindings && ts.isNamedImports(bindings)) for (const element of bindings.elements) names.push(element.name)
  for (const name of names) {
    const symbol = checker.getSymbolAtLocation(name)
    if (symbol) importAuthorityBySymbol.set(symbol, { package: externalPackage, resolution })
  }
}

function externalImportForExpression(node) {
  const checker = checkerForNode(node)
  if (!checker) return { package: "", resolution: "unresolved" }
  let current = node
  while (current) {
    if (ts.isIdentifier(current)) {
      const symbol = checker.getSymbolAtLocation(current)
      if (symbol && importAuthorityBySymbol.has(symbol)) return importAuthorityBySymbol.get(symbol)
    }
    if (ts.isPropertyAccessExpression(current)) current = current.expression
    else if (ts.isCallExpression(current)) current = current.expression
    else break
  }
  return { package: "", resolution: "unresolved" }
}

function packageForDeclarationFile(filename) {
  const nodeModulesRoot = path.join(root, "node_modules")
  const resolved = path.resolve(filename)
  const prefix = `${nodeModulesRoot}${path.sep}`
  if (!resolved.startsWith(prefix)) return ""
  const parts = slash(path.relative(nodeModulesRoot, resolved)).split("/")
  if (parts[0]?.startsWith("@") && parts[1]) return `${parts[0]}/${parts[1]}`
  return parts[0] || ""
}

function declarationPackages(symbol) {
  const packages = new Set()
  if (!symbol) return packages
  for (const declaration of symbolDeclarations(symbol)) {
    const packageName = packageForDeclarationFile(declaration.getSourceFile().fileName)
    if (packageName) packages.add(packageName)
  }
  return packages
}

function evidencePackages(node) {
  const packages = new Set()
  try {
    const checker = checkerForNode(node)
    if (!checker) return packages
    const symbol = checker.getSymbolAtLocation(node)
    for (const value of declarationPackages(symbol)) packages.add(value)
    const type = checker.getTypeAtLocation(node)
    for (const value of declarationPackages(typeof type?.getAliasSymbol === "function" ? type.getAliasSymbol() : type?.aliasSymbol)) packages.add(value)
    for (const value of declarationPackages(typeof type?.getSymbol === "function" ? type.getSymbol() : type?.symbol)) packages.add(value)
    const apparent = type ? checker.getApparentType(type) : undefined
    for (const value of declarationPackages(typeof apparent?.getAliasSymbol === "function" ? apparent.getAliasSymbol() : apparent?.aliasSymbol)) packages.add(value)
    for (const value of declarationPackages(typeof apparent?.getSymbol === "function" ? apparent.getSymbol() : apparent?.symbol)) packages.add(value)
  } catch {}
  return packages
}

function hasPackageEvidence(node, accepted) {
  const imported = externalImportForExpression(node)
  if (imported.resolution === "exact" && accepted.has(imported.package)) return true
  for (const packageName of evidencePackages(node)) if (accepted.has(packageName)) return true
  return false
}

const expressPackages = new Set(["express", "@types/express", "@types/express-serve-static-core"])
const nodeServerPackages = new Set(["@types/node"])

for (const { sourceFile } of sourceFiles) {
  const visit = (node) => {
    if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
      const specifier = node.moduleSpecifier.text
      const resolved = resolutionFor(specifier, sourceFile.fileName, node.moduleSpecifier)
      const value = {
        ref: factRef("import", node.moduleSpecifier, specifier), kind: node.importClause?.isTypeOnly ? "type_import" : "import",
        specifier, importer_file_ref: fileRefByPath.get(relative(sourceFile.fileName)), ...resolved, location: locationOf(node.moduleSpecifier),
      }
      imports.push(value)
      bindImportSymbols(node, value.external_package, value.resolution)
    }
    if (ts.isExportDeclaration(node)) {
      const specifier = node.moduleSpecifier && ts.isStringLiteral(node.moduleSpecifier) ? node.moduleSpecifier.text : ""
      const resolved = specifier ? resolutionFor(specifier, sourceFile.fileName, node.moduleSpecifier) : { resolution: "exact", resolved_file_ref: "", external_package: "" }
      const names = node.exportClause && ts.isNamedExports(node.exportClause) ? node.exportClause.elements.map((item) => item.name.text) : ["*"]
      for (const name of names) exports.push({
        ref: factRef("export", node, `${specifier}:${name}`), kind: specifier ? "reexport" : "export", name,
        from_specifier: specifier, resolved_file_ref: resolved.resolved_file_ref, resolution: resolved.resolution, location: locationOf(node),
      })
      if (specifier) imports.push({
        ref: factRef("import", node.moduleSpecifier, `reexport:${specifier}`), kind: "reexport", specifier,
        importer_file_ref: fileRefByPath.get(relative(sourceFile.fileName)), ...resolved, location: locationOf(node.moduleSpecifier),
      })
    }
    ts.forEachChild(node, visit)
  }
  visit(sourceFile)
}

for (const declaration of declarations) {
  if (!declaration.exported || declaration.kind === "module") continue
  exports.push({
    ref: `export:${declaration.ref}`, kind: "declaration", name: declaration.name,
    declaration_ref: declaration.ref, resolution: "exact", location: declaration.location,
  })
}

const calls = []
const routes = []
const httpUses = []
const contracts = []
const resources = []
const surfaces = []
const contractByDeclaration = new Map()
const resourceByPackage = new Map()
const declarationKindByRef = new Map()
for (const declaration of declarations) {
  if (declaration.kind === "module") continue
  declarationKindByRef.set(declaration.ref, declaration.kind)
}

const resourceKinds = new Map([
  ["express", ["http_server", "Express"]],
  ["react-dom", ["browser_dom", "React DOM"]],
  ["@tanstack/react-query", ["client_data_cache", "TanStack Query"]],
  ["wouter", ["browser_router", "Wouter"]],
  ["drizzle-orm", ["postgres_database", "PostgreSQL via Drizzle"]],
  ["pg", ["postgres_database", "PostgreSQL"]],
  ["connect-pg-simple", ["postgres_sessions", "PostgreSQL sessions"]],
  ["googleapis", ["google_calendar", "Google Calendar"]],
  ["passport-google-oauth20", ["google_oauth", "Google OAuth"]],
  ["node-cron", ["scheduler", "Cron scheduler"]],
])

for (const value of imports) {
  if (!value.external_package) continue
  const resource = resourceKinds.get(value.external_package)
  if (!resource) continue
  const key = `${resource[0]}:${value.external_package}`
  let fact = resourceByPackage.get(key)
  if (!fact) {
    fact = {
      ref: `resource:${stablePart(key)}`, kind: resource[0], name: resource[1], package_path: value.external_package,
      used_by_refs: [], evidence_refs: [], location: value.location,
    }
    resourceByPackage.set(key, fact)
    resources.push(fact)
  }
  fact.evidence_refs.push(value.ref)
  fact.used_by_refs.push(moduleRef(value.location.path))
}

function staticString(node) {
  if (!node) return ""
  if (ts.isStringLiteralLike(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text
  return ""
}
function propertyName(node) {
  if (ts.isPropertyAccessExpression(node)) return node.name.text
  if (ts.isIdentifier(node)) return node.text
  return ""
}
function queryKeysFrom(node) {
  if (!node || !ts.isObjectLiteralExpression(node)) return []
  const property = node.properties.find((item) => ts.isPropertyAssignment(item) && item.name && expressionText(item.name) === "queryKey")
  if (!property || !ts.isPropertyAssignment(property)) return []
  const initializer = property.initializer
  if (ts.isArrayLiteralExpression(initializer)) return initializer.elements.map(staticString).filter(Boolean)
  return []
}
function objectMethod(node) {
  if (!node || !ts.isObjectLiteralExpression(node)) return ""
  const property = node.properties.find((item) => ts.isPropertyAssignment(item) && expressionText(item.name) === "method")
  return property && ts.isPropertyAssignment(property) ? staticString(property.initializer).toUpperCase() : ""
}
function rootCallName(node) {
  let current = node
  while (ts.isCallExpression(current) || ts.isPropertyAccessExpression(current)) current = current.expression
  return ts.isIdentifier(current) ? current.text : ""
}
function isSupportingToolPath(filePath) {
  const lower = filePath.toLowerCase()
  return lower.split("/").some((part) => part === "test" || part === "tests" || part === "integration" || part === "__tests__") ||
    /(?:^|\/)[^/]+\.(?:test|spec)\.[cm]?[jt]sx?$/.test(lower)
}
function addContract(fact) {
  contracts.push(fact)
  if (fact.declaration_ref) contractByDeclaration.set(fact.declaration_ref, fact)
}
function expressionRefs(node) {
  const refs = refsForExpression(node)
  if (refs.length > 0) return refs
  if (ts.isCallExpression(node)) return refsForExpression(node.expression)
  return []
}

for (const { sourceFile, path: filePath } of sourceFiles) {
  const visitContracts = (node) => {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer) {
      const declarationRef = declarationRefByNode.get(node) || ""
      const rootName = rootCallName(node.initializer)
      if (rootName === "z" || rootName === "createInsertSchema" || expressionText(node.initializer).startsWith("z.")) {
        addContract({ ref: factRef("contract", node.name, `zod:${node.name.text}`), kind: "zod_schema", name: node.name.text, value: rootName === "createInsertSchema" ? "drizzle_zod" : "zod", declaration_ref: declarationRef, used_by_refs: [], location: locationOf(node.name) })
      }
      if (ts.isCallExpression(node.initializer) && ["pgTable", "sqliteTable", "mysqlTable"].includes(propertyName(node.initializer.expression))) {
        addContract({ ref: factRef("contract", node.name, `drizzle:${node.name.text}`), kind: "drizzle_table", name: node.name.text, value: staticString(node.initializer.arguments[0]), declaration_ref: declarationRef, used_by_refs: [], location: locationOf(node.name) })
      }
    }
    if ((ts.isTypeAliasDeclaration(node) || ts.isInterfaceDeclaration(node)) && declarationExported(node) && filePath.startsWith("shared/")) {
      addContract({ ref: factRef("contract", node.name, `shared:${node.name.text}`), kind: "shared_type", name: node.name.text, declaration_ref: declarationRefByNode.get(node) || "", used_by_refs: [], location: locationOf(node.name) })
    }
    if (ts.isPropertyAccessExpression(node) && ts.isPropertyAccessExpression(node.expression) && expressionText(node.expression) === "process.env") {
      addContract({ ref: factRef("contract", node.name, `env:${node.name.text}`), kind: "environment_variable", name: node.name.text, used_by_refs: [refForDeclarationNode(node)], location: locationOf(node.name) })
    }
    ts.forEachChild(node, visitContracts)
  }
  visitContracts(sourceFile)
}

for (const { sourceFile } of sourceFiles) {
  const visitUses = (node) => {
    if (ts.isIdentifier(node)) {
      const refs = refsForExpression(node)
      for (const ref of refs) {
        const contract = contractByDeclaration.get(ref)
        if (contract) contract.used_by_refs.push(refForDeclarationNode(node))
      }
    }
    ts.forEachChild(node, visitUses)
  }
  visitUses(sourceFile)
}

for (const { sourceFile } of sourceFiles) {
  const visit = (node) => {
    if (ts.isCallExpression(node) || ts.isNewExpression(node)) {
      const invocation = ts.isNewExpression(node) ? "construct" : "call"
      const callerRef = refForDeclarationNode(node)
      let localRefs = (ts.isNewExpression(node) ? localRefsForInvocation(node) : expressionRefs(node.expression))
        .filter((ref) => ["function", "method", "lambda"].includes(declarationKindByRef.get(ref)))
      const externalImport = localRefs.length === 0 ? externalImportForExpression(node.expression) : { package: "", resolution: "unresolved" }
      let externalPackage = externalImport.package
      let externalReceiver = ""
      let externalName = externalPackage ? propertyName(node.expression) : ""
      let platformTarget
      if (localRefs.length === 0 && externalPackage === "") {
        platformTarget = platformTargetForInvocation(node)
        if (platformTarget) {
          externalPackage = "platform:javascript"
          externalReceiver = platformTarget.receiver
          externalName = platformTarget.name
        }
      }
      // A property name is not program-call authority. In a large project,
      // mapping `value.test()` or `console.error()` to every local declaration
      // named `test` or `error` creates false edges and an unbounded candidate
      // set. Retain compiler/type-resolved local refs, exact external-import
      // authority, or an explicit unresolved frontier with no invented target.
      let resolution = localRefs.length > 1 ? "alternatives" : localRefs.length === 1 ? "exact" : platformTarget ? "exact" : externalPackage ? externalImport.resolution : "unresolved"
      if (/\.(?:js|jsx|mjs|cjs)$/.test(relative(sourceFile.fileName)) && resolution === "exact") resolution = "alternatives"
      const displayExpression = invocation === "construct" ? `new ${expressionText(node.expression)}` : expressionText(node.expression)
      const call = {
        ref: factRef("call", node, displayExpression), caller_ref: callerRef, callee_refs: localRefs,
        invocation, external_package: externalPackage, external_receiver: externalReceiver, external_name: externalName,
        expression: displayExpression, resolution, location: locationOf(node.expression),
      }
      calls.push(call)
      if (externalPackage) {
        for (const resource of resources) if (resource.package_path === externalPackage) resource.used_by_refs.push(callerRef)
      }

      if (ts.isCallExpression(node)) {
      const name = propertyName(node.expression)
      const method = name.toUpperCase()
      const frameworkReceiver = ts.isPropertyAccessExpression(node.expression) ? node.expression.expression : undefined
      if (["GET", "POST", "PUT", "PATCH", "DELETE"].includes(method) && frameworkReceiver && hasPackageEvidence(frameworkReceiver, expressPackages)) {
        const routePath = staticString(node.arguments[0])
        if (routePath) {
          const argumentRefs = []
          for (const argument of node.arguments.slice(1)) argumentRefs.push(...expressionRefs(argument))
          const distinct = [...new Set(argumentRefs)]
          routes.push({
            ref: factRef("route", node, `${method}:${routePath}`), kind: "http_route", method, path: routePath,
            owner_ref: callerRef, middleware_refs: distinct.slice(0, Math.max(0, distinct.length - 1)),
            handler_refs: distinct.slice(Math.max(0, distinct.length - 1)), resolution: distinct.length > 0 ? "exact" : "unresolved", location: locationOf(node),
          })
        }
      }
      if (name === "use" && frameworkReceiver && hasPackageEvidence(frameworkReceiver, expressPackages)) {
        const firstPath = staticString(node.arguments[0])
        const handlerArguments = firstPath ? node.arguments.slice(1) : node.arguments
        const refs = [...new Set(handlerArguments.flatMap(expressionRefs))]
        routes.push({
          ref: factRef("route", node, `USE:${firstPath || "*"}`), kind: "middleware", method: "USE", path: firstPath || "*",
          owner_ref: callerRef, middleware_refs: refs, handler_refs: [], resolution: refs.length ? "exact" : "unresolved", location: locationOf(node),
        })
      }
      if (name === "listen" && ts.isPropertyAccessExpression(node.expression) &&
          (hasPackageEvidence(node.expression.name, nodeServerPackages) || hasPackageEvidence(node.expression.expression, nodeServerPackages))) {
        const tool = isSupportingToolPath(relative(sourceFile.fileName))
        surfaces.push({ ref: factRef("surface", node, tool ? "node-server-tool" : "node-server"), kind: tool ? "tool" : "node_server", role: tool ? "script" : "product", name: tool ? "Integration/test HTTP server" : "Node HTTP server", entry_refs: [callerRef], evidence_refs: [call.ref], location: locationOf(node) })
      }
      if ((name === "createRoot" || expressionText(node.expression).endsWith(".createRoot")) &&
          externalImport.resolution === "exact" && externalPackage === "react-dom") {
        surfaces.push({ ref: factRef("surface", node, "browser"), kind: "browser_application", role: "product", name: "React browser application", entry_refs: [callerRef], evidence_refs: [call.ref], location: locationOf(node) })
      }
      if ((name === "useQuery" || name === "useMutation") && node.arguments[0] &&
          externalImport.resolution === "exact" && externalPackage === "@tanstack/react-query") {
        const keys = queryKeysFrom(node.arguments[0])
        if (keys.length > 0) addContract({ ref: factRef("contract", node, `query-key:${keys.join("/")}`), kind: "query_key", name: keys.join("/"), value: JSON.stringify(keys), used_by_refs: [callerRef], location: locationOf(node) })
      }
      if (name === "apiRequest") {
        const httpMethod = staticString(node.arguments[0]).toUpperCase()
        const requestPath = staticString(node.arguments[1])
        if (httpMethod && requestPath) httpUses.push({ ref: factRef("http-use", node, `${httpMethod}:${requestPath}`), kind: "api_request", method: httpMethod, path: requestPath, caller_ref: callerRef, query_keys: [], resolution: "exact", location: locationOf(node) })
      }
      if (name === "fetch") {
        const requestPath = staticString(node.arguments[0])
        if (requestPath) httpUses.push({ ref: factRef("http-use", node, `FETCH:${requestPath}`), kind: "fetch", method: objectMethod(node.arguments[1]) || "GET", path: requestPath, caller_ref: callerRef, query_keys: [], resolution: "exact", location: locationOf(node) })
      }
      if (name === "schedule" && externalImport.resolution === "exact" && externalPackage === "node-cron") {
        const schedule = staticString(node.arguments[0])
        if (schedule) addContract({ ref: factRef("contract", node, `cron:${schedule}`), kind: "cron_schedule", name: schedule, value: schedule, used_by_refs: [callerRef], location: locationOf(node) })
      }
      if (node.expression.kind === ts.SyntaxKind.ImportKeyword && node.arguments[0] && ts.isStringLiteral(node.arguments[0])) {
        const specifier = node.arguments[0].text
        imports.push({ ref: factRef("import", node, `dynamic:${specifier}`), kind: "dynamic_import", specifier, importer_file_ref: fileRefByPath.get(relative(sourceFile.fileName)), ...resolutionFor(specifier, sourceFile.fileName, node.arguments[0]), location: locationOf(node) })
      }
      }
    }

    if (ts.isJsxSelfClosingElement(node) || ts.isJsxOpeningElement(node)) {
      const tag = expressionText(node.tagName).split(".").at(-1)
      const jsxImport = externalImportForExpression(node.tagName)
      if (tag === "Route" && jsxImport.resolution === "exact" && jsxImport.package === "wouter") {
        let routePath = "*"
        let componentRef = ""
        for (const attribute of node.attributes.properties) {
          if (!ts.isJsxAttribute(attribute)) continue
          const attributeName = attribute.name.text
          if (attributeName === "path") {
            if (attribute.initializer && ts.isStringLiteral(attribute.initializer)) routePath = attribute.initializer.text
            else if (attribute.initializer && ts.isJsxExpression(attribute.initializer)) routePath = staticString(attribute.initializer.expression) || "*"
          }
          if (attributeName === "component" && attribute.initializer && ts.isJsxExpression(attribute.initializer) && attribute.initializer.expression) {
            componentRef = expressionRefs(attribute.initializer.expression)[0] || ""
          }
        }
        routes.push({ ref: factRef("route", node, `BROWSER:${routePath}`), kind: "browser_route", path: routePath, owner_ref: refForDeclarationNode(node), component_ref: componentRef, middleware_refs: [], handler_refs: [], resolution: componentRef ? "exact" : "unresolved", location: locationOf(node) })
      }
      if (tag === "Link" && jsxImport.resolution === "exact" && jsxImport.package === "wouter") {
        let linkPath = ""
        for (const attribute of node.attributes.properties) {
          if (!ts.isJsxAttribute(attribute) || (attribute.name.text !== "href" && attribute.name.text !== "to")) continue
          if (attribute.initializer && ts.isStringLiteral(attribute.initializer)) linkPath = attribute.initializer.text
          else if (attribute.initializer && ts.isJsxExpression(attribute.initializer)) linkPath = staticString(attribute.initializer.expression)
        }
        routes.push({
          ref: factRef("route", node, `LINK:${linkPath || "dynamic"}`), kind: "browser_link", path: linkPath || "<dynamic>",
          owner_ref: refForDeclarationNode(node), middleware_refs: [], handler_refs: [], resolution: linkPath ? "exact" : "unresolved", location: locationOf(node),
        })
      }
    }
    ts.forEachChild(node, visit)
  }
  visit(sourceFile)
}

if (contracts.some((value) => value.kind === "shared_type" || value.location.path.startsWith("shared/"))) {
  const first = contracts.find((value) => value.location.path.startsWith("shared/"))
  surfaces.push({ ref: "surface:shared-contracts", kind: "shared_contracts", role: "supporting", name: "Shared client/server contracts", entry_refs: [], evidence_refs: contracts.filter((value) => value.location.path.startsWith("shared/")).map((value) => value.ref), location: first.location })
}

const uniqueByRef = (values) => {
  const byRef = new Map()
  for (const value of values) if (!byRef.has(value.ref)) byRef.set(value.ref, value)
  return [...byRef.values()].sort((a, b) => compareText(a.ref, b.ref))
}
const canonicalStrings = (values) => [...new Set(values.filter(Boolean))].sort()
for (const value of calls) value.callee_refs = canonicalStrings(value.callee_refs)
for (const value of routes) {
  value.middleware_refs = canonicalStrings(value.middleware_refs)
  value.handler_refs = canonicalStrings(value.handler_refs)
}
for (const value of contracts) value.used_by_refs = canonicalStrings(value.used_by_refs)
for (const value of resources) {
  value.used_by_refs = canonicalStrings(value.used_by_refs)
  value.evidence_refs = canonicalStrings(value.evidence_refs)
}
for (const value of surfaces) {
  value.entry_refs = canonicalStrings(value.entry_refs)
  value.evidence_refs = canonicalStrings(value.evidence_refs)
}

const result = {
  helper_version: CONTRACT_VERSION,
  module_resolution: moduleResolution,
  base_url: baseURL,
  path_aliases: pathAliases,
  files,
  declarations: uniqueByRef(declarations),
  imports: uniqueByRef(imports),
  exports: uniqueByRef(exports),
  calls: uniqueByRef(calls),
  surfaces: uniqueByRef(surfaces),
  routes: uniqueByRef(routes),
  http_uses: uniqueByRef(httpUses),
  contracts: uniqueByRef(contracts),
  resources: uniqueByRef(resources),
}

const sourceDigest = createHash("sha256")
for (const file of files) {
  for (const field of [file.path, file.file_ref, file.sha256]) {
    sourceDigest.update(String(Buffer.byteLength(field)))
    sourceDigest.update("\0")
    sourceDigest.update(field)
  }
}
result.source_sha256 = sourceDigest.digest("hex")

const encodedResult = JSON.stringify(result)
if (nativeSnapshot) nativeSnapshot.dispose()
if (nativeAPI) nativeAPI.close()
process.stdout.write(encodedResult)
