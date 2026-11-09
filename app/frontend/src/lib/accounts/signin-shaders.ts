import { SEED_COLS, WAVE_BAND, WAVE_BLOCKS, WAVE_JITTER_MS, WAVE_MS, SWAP_BAND, SWAP_BLOCK, SWAP_JITTER_MS, SWAP_MS } from './signin-art.ts';

// The radial wave, in one fragment shader. Per pixel it finds its 36 px cell (with jitter) and its distance from the panel centre, which gives its level. Level 0 samples the 24-across frame-0 grid, levels 1 to 3 square screen blocks (mipmapped, so a block is the average of what it covers), and level 4 is clear so the sharp <img> underneath shows. waveLevel in signin-art.ts is the same maths, unit tested.
export const VERT = `#version 300 es
in vec2 a;
void main() { gl_Position = vec4(a, 0.0, 1.0); }`;
export const FRAG = `#version 300 es
precision highp float;
uniform sampler2D seedTex;
uniform sampler2D detTex;
uniform vec2 view;
uniform vec2 seedSize;
uniform vec2 detSize;
uniform vec2 pos;
uniform vec2 centre;
uniform float maxD;
uniform float dpr;
uniform float t;
out vec4 o;
const float WAVE = ${WAVE_MS.toFixed(1)};
const float BAND = ${WAVE_BAND.toFixed(3)};
const float JIT = ${WAVE_JITTER_MS.toFixed(1)};
const float COLS = ${SEED_COLS.toFixed(1)};
vec2 coverOrigin(vec2 s, out float k) { k = max(view.x / s.x, view.y / s.y); return (view - s * k) * pos; }
float hash(vec2 c) { return fract(sin(dot(c, vec2(127.1, 311.7))) * 43758.5453); }
vec3 block(vec2 p, float b) {
vec2 c = (floor(p / b) + 0.5) * b;
float k; vec2 org = coverOrigin(detSize, k);
vec2 uv = (c - org) / (detSize * k);
return textureLod(detTex, uv, max(0.0, log2(b * dpr / (k * dpr)))).rgb;
}
void main() {
// One canvas pixel is dpr CSS px square (3): every block size is a whole number of them.
vec2 p = vec2(gl_FragCoord.x, ceil(view.y / dpr) - gl_FragCoord.y) * dpr;
vec2 cell = floor(p / ${WAVE_BLOCKS[0].toFixed(1)});
float j = (hash(cell) * 2.0 - 1.0) * JIT;
float f = distance((cell + 0.5) * ${WAVE_BLOCKS[0].toFixed(1)}, centre) / maxD;
float e = clamp((t + j) / WAVE, 0.0, 1.0);
float pr = (1.0 - pow(1.0 - e, 3.0)) * (1.0 + BAND);
float s = clamp((pr - f) / BAND, 0.0, 1.0);
if (s >= 1.0) { o = vec4(0.0); return; }
float lv = floor(s * 4.0);
vec3 col;
if (lv < 1.0) {
	// The frame-0 grid: COLS blocks across the picture, cover-fit like app.html's background.
	float k; vec2 org = coverOrigin(seedSize, k);
	vec2 g = vec2(COLS, COLS * seedSize.y / seedSize.x);
	vec2 uv = (floor((p - org) / (seedSize * k) * g) + 0.5) / g;
	col = textureLod(seedTex, uv, max(0.0, log2(seedSize.x / COLS))).rgb;
} else {
	// One sampling path for the three square sizes: a third of the code for the driver to compile (D3D compiles it on the GPU thread).
	col = block(p, lv < 2.0 ? ${WAVE_BLOCKS[0].toFixed(1)} : lv < 3.0 ? ${WAVE_BLOCKS[1].toFixed(1)} : ${WAVE_BLOCKS[2].toFixed(1)});
}
o = vec4(col, 1.0);
}`;

// Later changes: the same mosaic language, softer and quicker. The new picture dissolves in over the old one through large (56 px), low-contrast blocks, in a wave from one edge or corner, over about 1.2 s on a quartic ease-out. Each block fades in as its own average colour, then resolves to the sharp picture underneath. The canvas sits in the new picture's Ken Burns box, so at the hand-over the <img> lands on the same pixels.
export const SWAP_FRAG = `#version 300 es
precision highp float;
uniform sampler2D tex;
uniform vec2 view;
uniform vec2 texSize;
uniform vec2 pos;
uniform vec2 origin;
uniform float maxD;
uniform float pr;
uniform float px;
out vec4 o;
const float B = ${SWAP_BLOCK.toFixed(1)};
const float BAND = ${SWAP_BAND.toFixed(3)};
float hash(vec2 c) { return fract(sin(dot(c, vec2(127.1, 311.7))) * 43758.5453); }
void main() {
vec2 p = vec2(gl_FragCoord.x, view.y * px - gl_FragCoord.y) / px;
float k = max(view.x / texSize.x, view.y / texSize.y);
vec2 org = (view - texSize * k) * pos;
vec2 cell = floor(p / B);
float f = distance((cell + 0.5) * B, origin) / maxD + (hash(cell) * 2.0 - 1.0) * ${(SWAP_JITTER_MS / SWAP_MS).toFixed(3)};
float s = clamp((pr * (1.0 + BAND) - f) / BAND, 0.0, 1.0);
if (s <= 0.0) { o = vec4(0.0); return; }
vec2 c = (cell + 0.5) * B;
vec3 blk = textureLod(tex, (c - org) / (texSize * k), max(0.0, log2(B / k))).rgb;
vec3 sharp = texture(tex, (p - org) / (texSize * k)).rgb;
// Low contrast: each block is its average pulled towards the sharp pixel, then it clears into the sharp picture.
vec3 col = mix(mix(blk, sharp, 0.35), sharp, smoothstep(0.45, 1.0, s));
float a = smoothstep(0.0, 0.5, s);
o = vec4(col * a, a);
}`;
