// Alloy Node Plugin Bridge
// Runs as a subprocess, communicates via JSON-RPC over stdin/stdout.
// Implements the alloy plugin API: alloy.filter(), alloy.shortcode(), alloy.hook()

const filters = {};
const shortcodes = {};
const hooks = {};

const alloy = {
  filter(name, fn) { filters[name] = fn; },
  shortcode(name, fn) { shortcodes[name] = fn; },
  hook(name, fn) { hooks[name] = fn; },
  on(name, fn) { hooks[name] = fn; },
};

function sendMessage(msg) {
  const body = JSON.stringify(msg);
  const frame = `Content-Length: ${Buffer.byteLength(body)}\r\n\r\n${body}`;
  process.stdout.write(frame);
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
        // Evaluate plugin source — wraps in a function and calls with alloy
        const src = msg.payload;
        // Strip "export const runtime = ..." and "export default function"
        let code = src.replace(/export\s+const\s+runtime\s*=\s*["']node["'];?\s*/g, '');
        code = code.replace(/export\s+default\s+function\s*\(\s*alloy\s*\)/, '(function(alloy)');
        code = code.trimEnd();
        if (!code.endsWith('(alloy);')) {
          code += ')(alloy);';
        }
        eval(code);
        sendMessage({
          id: msg.id,
          result: {
            filters: Object.keys(filters),
            shortcodes: Object.keys(shortcodes),
            hooks: Object.keys(hooks),
          },
        });
        break;
      }
      case 'filter': {
        const fn = filters[msg.name];
        if (!fn) { sendMessage({ id: msg.id, error: `filter "${msg.name}" not found` }); return; }
        const result = await fn(msg.payload);
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
        const result = await fn(msg.payload.args || [], msg.payload.content || '');
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
