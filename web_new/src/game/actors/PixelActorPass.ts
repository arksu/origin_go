import { Color, DepthTexture, Mesh, NearestFilter, OrthographicCamera, PlaneGeometry, RGBAFormat, Scene, ShaderMaterial, UnsignedByteType, UnsignedIntType, Vector2, WebGLRenderTarget, type WebGLRenderer } from 'three'
import { ACTOR_PALETTE, ACTOR_RENDER } from './config'

const palette = ACTOR_PALETTE.map((ramp) => ramp.map((hex) => {
  const color = new Color(hex).convertLinearToSRGB()
  return `vec3(${color.r.toFixed(6)},${color.g.toFixed(6)},${color.b.toFixed(6)})`
}))
const paletteShader = palette.map((ramp, region) => `if (region == ${region + 1}${region === 1 ? ' || region == 5' : ''}) { ${ramp.map((color) => `consider(${color}, color, closest, distance);`).join('\n')} }`).join('\n')

export class PixelActorPass {
  readonly source: WebGLRenderTarget
  readonly output: WebGLRenderTarget
  private readonly scene = new Scene()
  private readonly camera = new OrthographicCamera(-1, 1, 1, -1, 0, 1)
  private readonly geometry = new PlaneGeometry(2, 2)
  private readonly material: ShaderMaterial

  constructor() {
    const size = ACTOR_RENDER.cellSize
    this.source = new WebGLRenderTarget(size * ACTOR_RENDER.supersampling, size * ACTOR_RENDER.supersampling, {
      format: RGBAFormat, type: UnsignedByteType, minFilter: NearestFilter, magFilter: NearestFilter,
      depthTexture: new DepthTexture(size * ACTOR_RENDER.supersampling, size * ACTOR_RENDER.supersampling, UnsignedIntType),
      depthBuffer: true,
    })
    this.output = new WebGLRenderTarget(size, size, { format: RGBAFormat, type: UnsignedByteType, minFilter: NearestFilter, magFilter: NearestFilter, depthBuffer: false })
    this.material = new ShaderMaterial({
      depthTest: false, depthWrite: false,
      uniforms: {
        source: { value: this.source.texture },
        depth: { value: this.source.depthTexture },
        pixel: { value: new Vector2(1 / size, 1 / size) },
        hovered: { value: false },
      },
      vertexShader: 'varying vec2 pixelUV; void main() { pixelUV = uv; gl_Position = vec4(position.xy, 0.0, 1.0); }',
      fragmentShader: `
        uniform sampler2D source, depth;
        uniform vec2 pixel;
        uniform bool hovered;
        varying vec2 pixelUV;
        void consider(vec3 candidate, vec3 color, inout vec3 closest, inout float distance) {
          vec3 difference = candidate - color;
          float error = dot(difference * difference, vec3(2.0, 4.0, 3.0));
          if (error < distance) { distance = error; closest = candidate; }
        }
        vec3 quantize(vec3 color, int region) {
          float distance = 100.0;
          vec3 closest = color;
          ${paletteShader}
          return closest;
        }
        vec4 sampleCell(vec2 uv) {
          // Majority coverage keeps a fixed native grid while suppressing subpixel flicker.
          vec4 best = vec4(0.0);
          float weight = 0.0;
          for (int row = 0; row < 2; row++) for (int column = 0; column < 2; column++) {
            vec4 value = texture2D(source, uv + (vec2(float(column), float(row)) - 0.5) * pixel * 0.5);
            if (value.a > 0.01) { best.rgb += value.rgb; weight += 1.0; best.a = max(best.a, value.a); }
          }
          if (weight < 2.0) return vec4(0.0);
          return vec4(best.rgb / weight, best.a);
        }
        void main() {
          // Pixi interprets row zero as the top; flip once before the GPU copy.
          vec2 uv = vec2(pixelUV.x, 1.0 - pixelUV.y);
          vec4 center = sampleCell(uv);
          bool opaque = center.a > 0.01;
          bool outer = false, inner = false;
          float centerDepth = texture2D(depth, uv).r;
          float code = floor(center.a * 255.0 + 0.5);
          int region = int(floor(code / 32.0));
          float height = mod(code, 32.0) / 31.0 * 2.4;
          for (int row = -1; row <= 1; row++) for (int column = -1; column <= 1; column++) {
            if (row == 0 && column == 0) continue;
            vec2 neighbourUV = uv + vec2(float(column), float(row)) * pixel;
            vec4 neighbour = sampleCell(neighbourUV);
            bool otherOpaque = neighbour.a > 0.01;
            outer = outer || otherOpaque;
            if (opaque && otherOpaque && (row == 0 || column == 0)) {
              int otherRegion = int(floor((neighbour.a * 255.0 + 0.5) / 32.0));
              float difference = texture2D(depth, neighbourUV).r - centerDepth;
              inner = inner || (difference > 0.065 / 19.9 && (height < 1.40 || region == 2));
              inner = inner || ((region == 2 || region == 3) && otherRegion == 1);
            }
          }
          if (!opaque) {
            gl_FragColor = outer ? vec4(hovered ? vec3(1.0,0.88,0.46) : vec3(0.098,0.094,0.059), 1.0) : vec4(0.0);
          } else {
            vec3 color = quantize(center.rgb, region);
            if (inner) color = region == 2 ? vec3(0.098,0.094,0.059) : vec3(0.224,0.169,0.110);
            gl_FragColor = vec4(color, 1.0);
          }
        }`,
    })
    this.scene.add(new Mesh(this.geometry, this.material))
  }

  render(renderer: WebGLRenderer, hovered: boolean): void {
    this.material.uniforms.hovered!.value = hovered
    renderer.setRenderTarget(this.output)
    renderer.render(this.scene, this.camera)
  }

  destroy(): void {
    this.source.dispose()
    this.output.dispose()
    this.geometry.dispose()
    this.material.dispose()
  }
}
