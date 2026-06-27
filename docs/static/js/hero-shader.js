(function() {
  const canvas = document.getElementById('hero-shader');
  if (!canvas) return;
  const gl = canvas.getContext('webgl');
  if (!gl) return;

  const vs = `attribute vec2 a_pos;
void main() { gl_Position = vec4(a_pos, 0.0, 1.0); }`;

  const fs = `precision mediump float;
uniform float u_time;
uniform vec2 u_res;
uniform vec2 u_mouse;

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

  vec2 mDir = u_mouse - uv;
  float mDist = length(mDir);
  float mInfluence = smoothstep(0.5, 0.0, mDist) * 0.15;

  float warp1 = fbm(uv * 3.0 + vec2(t * 0.5, t * 0.3));
  float warp2 = fbm(uv * 3.0 + vec2(-t * 0.4, t * 0.6) + 4.0);
  vec2 warped = uv + vec2(warp1, warp2) * 0.3 + mDir * mInfluence;

  float f1 = fbm(warped * 2.5 + vec2(t * 0.3, -t * 0.2));
  float f2 = fbm(warped * 3.0 + vec2(-t * 0.5, t * 0.4) + 8.0);

  float blend = smoothstep(0.3, 0.7, f1);
  float highlight = smoothstep(0.55, 0.65, f2);

  vec3 blue   = vec3(0.376, 0.647, 0.980);
  vec3 steel  = vec3(0.44, 0.50, 0.56);
  vec3 cyan   = vec3(0.369, 0.720, 0.876);

  vec3 col = mix(blue, steel, blend);
  col = mix(col, cyan, highlight * 0.4);

  col += mInfluence * 0.4 * blue;

  float intensity = smoothstep(0.2, 0.8, fbm(warped * 2.0 + t * 0.15));
  col *= intensity;

  float vignette = 1.0 - length((uv - 0.5) * 1.4);
  vignette = smoothstep(0.0, 0.7, vignette);
  col *= vignette;

  gl_FragColor = vec4(col, 1.0);
}`;

  function compile(type, src) {
    var s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    return s;
  }

  var prog = gl.createProgram();
  gl.attachShader(prog, compile(gl.VERTEX_SHADER, vs));
  gl.attachShader(prog, compile(gl.FRAGMENT_SHADER, fs));
  gl.linkProgram(prog);
  gl.useProgram(prog);

  var buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1,1,-1,-1,1,1,1]), gl.STATIC_DRAW);
  var aPos = gl.getAttribLocation(prog, 'a_pos');
  gl.enableVertexAttribArray(aPos);
  gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

  var uTime = gl.getUniformLocation(prog, 'u_time');
  var uRes = gl.getUniformLocation(prog, 'u_res');
  var uMouse = gl.getUniformLocation(prog, 'u_mouse');

  var w, h, dpr, raf;
  var mouseTarget = [0.5, 0.5];
  var mouseCurrent = [0.5, 0.5];

  function resize() {
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    w = canvas.clientWidth;
    h = canvas.clientHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.uniform2f(uRes, canvas.width, canvas.height);
  }

  var hero = canvas.closest('.hero');
  hero.addEventListener('mousemove', function(e) {
    var rect = hero.getBoundingClientRect();
    mouseTarget[0] = (e.clientX - rect.left) / rect.width;
    mouseTarget[1] = 1.0 - (e.clientY - rect.top) / rect.height;
  });
  hero.addEventListener('mouseleave', function() {
    mouseTarget[0] = 0.5;
    mouseTarget[1] = 0.5;
  });

  var ro = new ResizeObserver(resize);
  ro.observe(canvas);
  resize();

  function frame(t) {
    mouseCurrent[0] += (mouseTarget[0] - mouseCurrent[0]) * 0.05;
    mouseCurrent[1] += (mouseTarget[1] - mouseCurrent[1]) * 0.05;
    gl.uniform2f(uMouse, mouseCurrent[0], mouseCurrent[1]);
    gl.uniform1f(uTime, t * 0.001);
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
