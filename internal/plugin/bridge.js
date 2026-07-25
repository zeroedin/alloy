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
const sources = {};
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
  source(name, fn) {
    if (Object.hasOwn(sources, name)) {
      warnings.push(`duplicate source registration: "${name}" registered multiple times, last registration wins`);
    }
    sources[name] = fn;
  },
};

function sendMessage(msg, requestType) {
  // For hook results with html or content string fields, use split-body framing
  // to avoid JSON-encoding the large string (issue #1181).
  if (requestType === 'hook' && msg.result && typeof msg.result === 'object') {
    const splitField = typeof msg.result.html === 'string' ? 'html'
      : typeof msg.result.content === 'string' ? 'content'
      : null;
    if (splitField && msg.result[splitField].length > 0) {
      const rawValue = msg.result[splitField];
      const clone = Object.assign({}, msg.result);
      delete clone[splitField];
      const stripped = Object.assign({}, msg, { result: clone });
      const jsonBody = JSON.stringify(stripped);
      const jsonLen = Buffer.byteLength(jsonBody);
      const rawLen = Buffer.byteLength(rawValue);
      const header = `Content-Length: ${jsonLen}\r\nX-Body-Length: ${rawLen}\r\nX-Body-Field: ${splitField}\r\n\r\n`;
      realStdoutWrite(header);
      realStdoutWrite(jsonBody);
      realStdoutWrite(rawValue);
      return;
    }
  }
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
    const clMatch = header.match(/Content-Length:\s*(\d+)/);
    if (!clMatch) { buffer = buffer.slice(headerEnd + 4); continue; }

    const contentLen = parseInt(clMatch[1], 10);
    const bodyStart = headerEnd + 4;

    // Parse optional split-body headers (issue #1181)
    const blMatch = header.match(/X-Body-Length:\s*(\d+)/);
    const bfMatch = header.match(/X-Body-Field:\s*(\S+)/);
    const splitBodyLen = blMatch ? parseInt(blMatch[1], 10) : 0;
    const splitBodyField = bfMatch ? bfMatch[1] : null;

    const totalLen = contentLen + splitBodyLen;
    if (buffer.length < bodyStart + totalLen) break;

    const jsonBody = buffer.slice(bodyStart, bodyStart + contentLen).toString('utf8');
    let rawBody = null;
    if (splitBodyLen > 0 && splitBodyField) {
      rawBody = buffer.slice(bodyStart + contentLen, bodyStart + totalLen).toString('utf8');
    }
    buffer = buffer.slice(bodyStart + totalLen);

    try {
      const msg = JSON.parse(jsonBody);
      // Inject split-body field into payload or result
      if (rawBody !== null && splitBodyField) {
        if (msg.payload && typeof msg.payload === 'object') {
          msg.payload[splitBodyField] = rawBody;
        } else if (msg.result && typeof msg.result === 'object') {
          msg.result[splitBodyField] = rawBody;
        }
      }
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
            sources: Object.keys(sources),
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
        sendMessage({ id: msg.id, result }, 'hook');
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
      case 'source': {
        const fn = sources[msg.name];
        if (!fn) { sendMessage({ id: msg.id, error: `source "${msg.name}" not found` }); return; }
        const result = await fn(msg.payload);
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
