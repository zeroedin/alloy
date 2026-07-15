// Alloy Node Plugin Bridge
// Runs as a subprocess, communicates via JSON-RPC over stdin/stdout.
// Implements the alloy plugin API: alloy.filter(), alloy.shortcode(), alloy.hook()

import { pathToFileURL } from 'node:url';

// Capture the real stdout.write before any plugin code can run.
// sendMessage is the ONLY code path that uses it — everything else
// (plugin code, npm dependencies, console.*) sees the redirected version
// that writes to stderr, keeping the JSON-RPC framing on stdout clean.
const realStdoutWrite = process.stdout.write.bind(process.stdout);
process.stdout.write = (chunk, encoding, callback) =>
  process.stderr.write(chunk, encoding, callback);

// Redirect console output to stderr so plugin console.log() doesn't corrupt the JSON-RPC framing on stdout.
const origConsole = { log: console.log, warn: console.warn, error: console.error, info: console.info, debug: console.debug };
console.log = (...args) => process.stderr.write(args.join(' ') + '\n');
console.warn = (...args) => process.stderr.write(args.join(' ') + '\n');
console.info = (...args) => process.stderr.write(args.join(' ') + '\n');
console.debug = (...args) => process.stderr.write(args.join(' ') + '\n');

const filters = {};
const shortcodes = {};
const hooks = {};
const hookScopes = {};
const warnings = [];

const alloy = {
  filter(name, fn) { filters[name] = fn; },
  shortcode(name, fn) { shortcodes[name] = fn; },
  hook(name, options, fn) {
    if (typeof options === 'function') {
      throw new Error('alloy.hook() requires options object as second argument: alloy.hook(name, { pages: true }, fn)');
    }
    if (typeof fn !== 'function') {
      throw new Error('alloy.hook() requires a function as third argument: alloy.hook(name, options, fn)');
    }
    if (!options || typeof options !== 'object') { options = {}; }
    if (Object.hasOwn(hooks, name)) {
      warnings.push(`duplicate hook registration: "${name}" registered multiple times, last registration wins`);
    }
    hooks[name] = fn;
    hookScopes[name] = {
      data: options.data !== undefined ? options.data : null,
      pages: options.pages !== undefined ? options.pages : null,
      pageFields: options.pageFields !== undefined ? options.pageFields : null,
      priority: (typeof options.priority === 'number') ? options.priority : 50,
    };
  },
  on(name, options, fn) { alloy.hook(name, options, fn); },
};

function sendMessage(msg) {
  const body = JSON.stringify(msg);
  const frame = `Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`;
  realStdoutWrite(frame);
}

let buffer = Buffer.alloc(0);

process.stdin.on('data', (chunk) => {
  buffer = Buffer.concat([buffer, chunk]);
  while (true) {
    const headerEnd = buffer.indexOf('\r\n\r\n');
    if (headerEnd < 0) break;

    const header = buffer.slice(0, headerEnd).toString('utf8');
    const match = header.match(/Content-Length:\s*(\d+)/);
    if (!match) { buffer = buffer.slice(headerEnd + 4); continue; }

    const len = parseInt(match[1], 10);
    const bodyStart = headerEnd + 4;
    if (buffer.length < bodyStart + len) break;

    const body = buffer.slice(bodyStart, bodyStart + len).toString('utf8');
    buffer = buffer.slice(bodyStart + len);

    try {
      const msg = JSON.parse(body);
      handleMessage(msg);
    } catch (e) {
      sendMessage({ id: 0, error: e.message });
    }
  }
});

async function handleMessage(msg) {
  try {
    switch (msg.type) {
      case 'eval': {
        const pluginPath = msg.payload;
        let mod;
        try {
          mod = await import(pathToFileURL(pluginPath).href);
        } catch (importErr) {
          throw new Error(
            `failed to import plugin ${pluginPath}: ${importErr.message}. ` +
            `Tier 3 plugins must be ESM — ensure the project has "type": "module" in package.json ` +
            `or use a .mjs extension.`
          );
        }
        if (typeof mod.default !== 'function') {
          throw new Error('plugin module must export a default function');
        }
        await mod.default(alloy);
        sendMessage({
          id: msg.id,
          result: {
            filters: Object.keys(filters),
            shortcodes: Object.keys(shortcodes),
            hooks: Object.keys(hooks),
            hookScopes: hookScopes,
            warnings: warnings.splice(0),
          },
        });
        break;
      }
      case 'filter': {
        const fn = filters[msg.name];
        if (!fn) { sendMessage({ id: msg.id, error: `filter "${msg.name}" not found` }); return; }
        const input = msg.payload && msg.payload.input !== undefined ? msg.payload.input : msg.payload;
        const args = msg.payload && Array.isArray(msg.payload.args) ? msg.payload.args : [];
        const result = await fn(input, ...args);
        sendMessage({ id: msg.id, result });
        break;
      }
      case 'hook': {
        const fn = hooks[msg.name];
        if (!fn) { sendMessage({ id: msg.id, error: `hook "${msg.name}" not found` }); return; }
        const result = await fn(msg.payload);
        sendMessage({ id: msg.id, result });
        break;
      }
      case 'shortcode': {
        const fn = shortcodes[msg.name];
        if (!fn) { sendMessage({ id: msg.id, error: `shortcode "${msg.name}" not found` }); return; }
        const scArgs = (msg.payload && msg.payload.args) || [];
        const scContent = (msg.payload && msg.payload.content) || '';
        const result = await fn(scArgs, scContent);
        sendMessage({ id: msg.id, result });
        break;
      }
      default:
        sendMessage({ id: msg.id, error: `unknown message type: ${msg.type}` });
    }
  } catch (e) {
    sendMessage({ id: msg.id, error: e.message });
  }
}
