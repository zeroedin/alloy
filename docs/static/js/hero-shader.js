(function() {
  const canvas = document.getElementById('hero-shader');
  if (!canvas) return;
  const gl = canvas.getContext('webgl');
  if (!gl) return;

  // ── Shared vertex shader ──────────────────────────
  const vs = `attribute vec2 a_pos;
void main() { gl_Position = vec4(a_pos, 0.0, 1.0); }`;

  // ── Pass 1: accumulate displacement into a buffer ─
  // Reads previous frame's displacement, decays it, adds new mouse force.
  const dispFS = `precision mediump float;
uniform sampler2D u_prev;
uniform vec2 u_res;
uniform vec2 u_mouse;
uniform vec2 u_mouse_vel;

void main() {
  vec2 uv = gl_FragCoord.xy / u_res;

  // Read previous displacement and decay it slowly
  vec2 prev = texture2D(u_prev, uv).xy * 2.0 - 1.0;
  prev *= 0.985;

  // Add new displacement from mouse velocity
  vec2 toMouse = uv - u_mouse;
  float dist = length(toMouse);
  float proximity = smoothstep(0.35, 0.0, dist);

  vec2 force = u_mouse_vel * proximity * 0.3;

  // Curl at edges: perpendicular push at the boundary
  vec2 curlDir = vec2(-u_mouse_vel.y, u_mouse_vel.x);
  float edge = smoothstep(0.0, 0.15, dist) * smoothstep(0.4, 0.12, dist);
  force += curlDir * edge * 0.08;

  vec2 total = prev + force;

  // Pack signed displacement into 0-1 range for texture storage
  gl_FragColor = vec4(total * 0.5 + 0.5, 0.0, 1.0);
}`;

  // ── Pass 2: render fog using accumulated displacement ─
  const fogFS = `precision mediump float;
uniform float u_time;
uniform vec2 u_res;
uniform sampler2D u_disp;

float noise(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453);
}

float smooth_noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  float a = noise(i);
  float b = noise(i + vec2(1.0, 0.0));
  float c = noise(i + vec2(0.0, 1.0));
  float d = noise(i + vec2(1.0, 1.0));
  return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float fbm(vec2 p) {
  float v = 0.0, a = 0.5;
  mat2 rot = mat2(0.8, 0.6, -0.6, 0.8);
  for (int i = 0; i < 5; i++) {
    v += a * smooth_noise(p);
    p = rot * p * 2.0;
    a *= 0.5;
  }
  return v;
}

void main() {
  vec2 uv = gl_FragCoord.xy / u_res;
  float t = u_time * 0.04;

  // Read accumulated displacement
  vec2 disp = texture2D(u_disp, uv).xy * 2.0 - 1.0;

  // Displace the fog sampling coordinates
  vec2 displaced = uv + disp * 0.15;

  float warp1 = fbm(displaced * 3.0 + vec2(t * 0.5, t * 0.3));
  float warp2 = fbm(displaced * 3.0 + vec2(-t * 0.4, t * 0.6) + 4.0);
  vec2 warped = displaced + vec2(warp1, warp2) * 0.3;

  float f1 = fbm(warped * 2.5 + vec2(t * 0.3, -t * 0.2));
  float f2 = fbm(warped * 3.0 + vec2(-t * 0.5, t * 0.4) + 8.0);

  float blend = smoothstep(0.3, 0.7, f1);
  float highlight = smoothstep(0.55, 0.65, f2);

  vec3 blue   = vec3(0.376, 0.647, 0.980);
  vec3 steel  = vec3(0.44, 0.50, 0.56);
  vec3 cyan   = vec3(0.369, 0.720, 0.876);

  vec3 col = mix(blue, steel, blend);
  col = mix(col, cyan, highlight * 0.4);

  float intensity = smoothstep(0.2, 0.8, fbm(warped * 2.0 + t * 0.15));
  col *= intensity;

  float vignette = 1.0 - length((uv - 0.5) * 1.4);
  vignette = smoothstep(0.0, 0.7, vignette);
  col *= vignette * 0.9;

  gl_FragColor = vec4(col, 1.0);
}`;

  // ── WebGL setup ───────────────────────────────────

  function compile(type, src) {
    var s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    return s;
  }

  function link(fsSrc) {
    var p = gl.createProgram();
    gl.attachShader(p, compile(gl.VERTEX_SHADER, vs));
    gl.attachShader(p, compile(gl.FRAGMENT_SHADER, fsSrc));
    gl.linkProgram(p);
    return p;
  }

  var dispProg = link(dispFS);
  var fogProg = link(fogFS);

  // Fullscreen quad
  var buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1,1,-1,-1,1,1,1]), gl.STATIC_DRAW);

  function bindQuad(prog) {
    var aPos = gl.getAttribLocation(prog, 'a_pos');
    gl.enableVertexAttribArray(aPos);
    gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);
  }

  // Ping-pong framebuffers for displacement accumulation
  function createFBO(w, h) {
    var tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, tex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    var fb = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, fb);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    return { fb: fb, tex: tex };
  }

  var dispW = 256, dispH = 256;
  var fboA = createFBO(dispW, dispH);
  var fboB = createFBO(dispW, dispH);
  var readFBO = fboA, writeFBO = fboB;

  // Uniform locations
  var dU = {
    prev:  gl.getUniformLocation(dispProg, 'u_prev'),
    res:   gl.getUniformLocation(dispProg, 'u_res'),
    mouse: gl.getUniformLocation(dispProg, 'u_mouse'),
    vel:   gl.getUniformLocation(dispProg, 'u_mouse_vel')
  };
  var fU = {
    time: gl.getUniformLocation(fogProg, 'u_time'),
    res:  gl.getUniformLocation(fogProg, 'u_res'),
    disp: gl.getUniformLocation(fogProg, 'u_disp')
  };

  var w, h, dpr, raf;
  var mouseTarget = [0.5, 0.5];
  var mouseCurrent = [0.5, 0.5];
  var mousePrev = [0.5, 0.5];
  var mouseVel = [0.0, 0.0];

  function resize() {
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    w = canvas.clientWidth;
    h = canvas.clientHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
  }

  var hero = canvas.closest('.hero');
  function updateTarget(x, y) {
    var rect = hero.getBoundingClientRect();
    mouseTarget[0] = (x - rect.left) / rect.width;
    mouseTarget[1] = 1.0 - (y - rect.top) / rect.height;
  }
  hero.addEventListener('mousemove', function(e) {
    updateTarget(e.clientX, e.clientY);
  });
  hero.addEventListener('mouseleave', function() {
    mouseTarget[0] = 0.5;
    mouseTarget[1] = 0.5;
  });
  hero.addEventListener('touchmove', function(e) {
    var touch = e.touches[0];
    updateTarget(touch.clientX, touch.clientY);
  }, { passive: true });
  hero.addEventListener('touchend', function() {
    mouseTarget[0] = 0.5;
    mouseTarget[1] = 0.5;
  });

  var ro = new ResizeObserver(resize);
  ro.observe(canvas);
  resize();

  function frame(t) {
    mousePrev[0] = mouseCurrent[0];
    mousePrev[1] = mouseCurrent[1];
    mouseCurrent[0] += (mouseTarget[0] - mouseCurrent[0]) * 0.06;
    mouseCurrent[1] += (mouseTarget[1] - mouseCurrent[1]) * 0.06;
    mouseVel[0] += ((mouseCurrent[0] - mousePrev[0]) - mouseVel[0]) * 0.08;
    mouseVel[1] += ((mouseCurrent[1] - mousePrev[1]) - mouseVel[1]) * 0.08;

    // Pass 1: update displacement buffer
    gl.bindFramebuffer(gl.FRAMEBUFFER, writeFBO.fb);
    gl.viewport(0, 0, dispW, dispH);
    gl.useProgram(dispProg);
    bindQuad(dispProg);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, readFBO.tex);
    gl.uniform1i(dU.prev, 0);
    gl.uniform2f(dU.res, dispW, dispH);
    gl.uniform2f(dU.mouse, mouseCurrent[0], mouseCurrent[1]);
    gl.uniform2f(dU.vel, mouseVel[0] * 60.0, mouseVel[1] * 60.0);
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);

    // Swap ping-pong
    var tmp = readFBO;
    readFBO = writeFBO;
    writeFBO = tmp;

    // Pass 2: render fog to screen
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.useProgram(fogProg);
    bindQuad(fogProg);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, readFBO.tex);
    gl.uniform1i(fU.disp, 0);
    gl.uniform2f(fU.res, canvas.width, canvas.height);
    gl.uniform1f(fU.time, t * 0.001);
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);

    raf = requestAnimationFrame(frame);
  }

  var mq = window.matchMedia('(prefers-reduced-motion: reduce)');
  if (!mq.matches) {
    raf = requestAnimationFrame(frame);
  }
  mq.addEventListener('change', function(e) {
    if (e.matches) {
      cancelAnimationFrame(raf);
      canvas.style.display = 'none';
    } else {
      canvas.style.display = '';
      raf = requestAnimationFrame(frame);
    }
  });
})();
