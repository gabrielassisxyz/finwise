// Writes internal/contract/contract.json: every method of FinwiseApi with its route, class,
// argument schemas and result schema, plus the error envelope. A reader that is not TypeScript
// (the Go backend) reads this file instead of the interface.
//
// ts-json-schema-generator cannot read the interface itself: a mapped or conditional type over
// FinwiseApi comes out as an object with no properties, because the methods are dropped. So the
// methods are walked with the TypeScript compiler API and each argument type and each result type
// is handed to the generator as an exported alias of a scratch file; the generator is only ever
// asked about named and inline types.
import { createRequire } from 'node:module';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const { createGenerator } = require('ts-json-schema-generator');

const webDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = resolve(webDir, '..');
const typesPath = join(webDir, 'src/api/types.ts');
const classificationPath = join(webDir, 'src/api/contract/classification.ts');
const tsconfigPath = join(webDir, 'tsconfig.app.json');
const defaultOutput = join(repoRoot, 'internal/contract/contract.json');

export const FORMAT_VERSION = 1;
const DEFINITIONS_REF = '#/definitions/';

function loadProgram() {
  const config = ts.readConfigFile(tsconfigPath, ts.sys.readFile);
  const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, webDir);
  return ts.createProgram([typesPath], parsed.options);
}

/** Domains and methods in declaration order, each with the source text of its argument and result types. */
export function readMethods(program) {
  const checker = program.getTypeChecker();
  const source = program.getSourceFile(typesPath);
  const root = source.statements.find((node) => ts.isInterfaceDeclaration(node) && node.name.text === 'FinwiseApi');
  const exported = new Set(
    checker.getExportsOfModule(checker.getSymbolAtLocation(source)).map((symbol) => symbol.getName()),
  );
  const methods = [];
  for (const domain of checker.getPropertiesOfType(checker.getTypeAtLocation(root))) {
    const domainType = checker.getTypeOfSymbolAtLocation(domain, root);
    for (const method of checker.getPropertiesOfType(domainType)) {
      const declaration = method.declarations.find(ts.isMethodSignature);
      if (!declaration) throw new Error(`${domain.name}.${method.name} is not a method signature`);
      methods.push({
        domain: domain.name,
        name: method.name,
        parameters: declaration.parameters.map((parameter) => ({
          name: parameter.name.getText(),
          optional: parameter.questionToken !== undefined,
          node: parameter.type,
        })),
        returns: promisedType(declaration),
      });
    }
  }
  return { methods, exported };
}

function promisedType(declaration) {
  const node = declaration.type;
  const wrapped = node && ts.isTypeReferenceNode(node) && node.typeName.getText() === 'Promise';
  if (!wrapped || node.typeArguments?.length !== 1) {
    throw new Error(`${declaration.getText()} must return Promise<T>`);
  }
  return node.typeArguments[0];
}

/** The names a type's source text refers to, so the scratch file can import exactly those. */
function referencedNames(node, exported, into) {
  const visit = (child) => {
    if (ts.isTypeReferenceNode(child)) {
      const name = child.typeName.getText().split('.')[0];
      if (exported.has(name)) into.add(name);
    }
    ts.forEachChild(child, visit);
  };
  visit(node);
}

/** The scratch module: one alias per argument, one per result, and the error envelope. */
function scratchSource(methods, exported) {
  const names = new Set(['ApiErrorEnvelope']);
  const aliases = [];
  for (const method of methods) {
    const key = `${method.domain}__${method.name}`;
    method.parameters.forEach((parameter, index) => {
      referencedNames(parameter.node, exported, names);
      aliases.push(`export type Arg__${key}__${index} = ${parameter.node.getText()};`);
    });
    referencedNames(method.returns, exported, names);
    if (method.returns.kind !== ts.SyntaxKind.VoidKeyword) {
      aliases.push(`export type Result__${key} = ${method.returns.getText()};`);
    }
  }
  aliases.push('export type Error__envelope = ApiErrorEnvelope;');
  const imports = `import type { ${[...names].sort().join(', ')} } from ${JSON.stringify(typesPath.replace(/\.ts$/, ''))};`;
  const aliasNames = aliases.map((line) => line.split(' ')[2]);
  return { text: `${imports}\n${aliases.join('\n')}\n`, aliasNames };
}

function generateDefinitions(methods, exported) {
  const dir = mkdtempSync(join(tmpdir(), 'finwise-contract-'));
  try {
    const path = join(dir, 'scratch.ts');
    const { text, aliasNames } = scratchSource(methods, exported);
    writeFileSync(path, text);
    const generator = createGenerator({
      path,
      tsconfig: tsconfigPath,
      type: '*',
      expose: 'all',
      topRef: true,
      additionalProperties: false,
      skipTypeCheck: false,
    });
    const definitions = {};
    for (const alias of aliasNames) {
      for (const [name, schema] of Object.entries(generator.createSchema(alias).definitions)) {
        const known = JSON.stringify(definitions[name] ?? schema);
        if (known !== JSON.stringify(schema)) throw new Error(`definition ${name} came out differently for ${alias}`);
        definitions[name] = schema;
      }
    }
    return definitions;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

const NULL = { type: 'null' };

function argumentsSchema(method, definitions) {
  const items = method.parameters.map((parameter, index) => {
    const schema = definitions[`Arg__${method.domain}__${method.name}__${index}`];
    return parameter.optional ? { anyOf: [schema, NULL] } : schema;
  });
  const required = method.parameters.filter((parameter) => !parameter.optional).length;
  const schema = { type: 'array' };
  if (items.length > 0) schema.items = items;
  if (required > 0) schema.minItems = required;
  schema.maxItems = items.length;
  if (items.length > 0) schema.additionalItems = false;
  return schema;
}

function resultSchema(method, definitions) {
  if (method.returns.kind === ts.SyntaxKind.VoidKeyword) return NULL;
  return definitions[`Result__${method.domain}__${method.name}`];
}

function sortedObject(object) {
  return Object.fromEntries(Object.keys(object).sort().map((key) => [key, object[key]]));
}

/** The file's content, as a string with a trailing newline; the same inputs always give the same bytes. */
export async function buildManifest() {
  const program = loadProgram();
  const { methods, exported } = readMethods(program);
  const classes = await loadClassification();
  const definitions = generateDefinitions(methods, exported);
  const entries = methods.map((method) => {
    const cls = classes[method.domain]?.[method.name];
    if (cls === undefined) throw new Error(`${method.domain}.${method.name} has no class in classification.ts`);
    return {
      domain: method.domain,
      name: method.name,
      route: `/api/${method.domain}/${method.name}`,
      class: cls,
      arguments: argumentsSchema(method, definitions),
      result: resultSchema(method, definitions),
    };
  });
  const shared = Object.fromEntries(Object.entries(definitions).filter(([key]) => !/^(Arg|Result|Error)__/.test(key)));
  const manifest = {
    formatVersion: FORMAT_VERSION,
    description:
      'Generated by web/scripts/contract-manifest.mjs; never edited by hand. A $ref resolves against this document: #/definitions/<Name>.',
    methods: entries,
    error: definitions.Error__envelope,
    definitions: sortedObject(shared),
  };
  return `${JSON.stringify(manifest, null, 2)}\n`;
}

/** classification.ts holds only types and a constant, so stripping the types is all it takes to read it. */
async function loadClassification() {
  const text = readFileSync(classificationPath, 'utf8');
  const { outputText } = ts.transpileModule(text, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } });
  const url = `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`;
  return (await import(url)).METHOD_CLASSES;
}

/** A schema that accepts anything, however it is spelled: `{}`, `true`, or only annotations. */
export function acceptsAnything(schema) {
  if (schema === true) return true;
  if (typeof schema !== 'object' || schema === null) return false;
  const annotations = new Set(['description', 'title', '$comment', 'default', 'examples', '$schema', '$id']);
  return Object.keys(schema).every((key) => annotations.has(key));
}

/** Every sub-schema that is empty, as a JSON path; the manifest must have none. */
export function findEmptySchemas(node, path = '$') {
  if (Array.isArray(node)) return node.flatMap((child, index) => findEmptySchemas(child, `${path}[${index}]`));
  if (typeof node !== 'object' || node === null) return [];
  const found = acceptsAnything(node) ? [path] : [];
  const holdsSchemas = ['properties', 'definitions'];
  const children = Object.entries(node).flatMap(([key, child]) => {
    if (holdsSchemas.includes(key)) {
      return Object.entries(child).flatMap(([name, sub]) => findEmptySchemas(sub, `${path}.${key}.${name}`));
    }
    return findEmptySchemas(child, `${path}.${key}`);
  });
  return [...found, ...children];
}

async function main(argv) {
  const check = argv.includes('--check');
  const outFlag = argv.indexOf('--out');
  const output = outFlag >= 0 ? resolve(argv[outFlag + 1]) : defaultOutput;
  const generated = await buildManifest();
  const label = relative(repoRoot, output);
  if (check) {
    let onDisk = null;
    try {
      onDisk = readFileSync(output, 'utf8');
    } catch {
      // A missing file is as stale as a different one.
    }
    if (onDisk === generated) return 0;
    console.error(`  ${label} is stale or missing: run \`npm --prefix web run contract\` and commit it`);
    return 1;
  }
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, generated);
  return 0;
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  main(process.argv.slice(2)).then(
    (code) => process.exit(code),
    (error) => {
      console.error(error);
      process.exit(1);
    },
  );
}
