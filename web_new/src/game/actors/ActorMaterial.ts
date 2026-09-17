import { Color, DoubleSide, MeshStandardMaterial, ShaderMaterial, Vector3, type Material } from 'three'
import type { DualQuaternionSkin } from './DualQuaternionSkin'
import { DQ_DECLARATIONS } from './DualQuaternionSkin'

export function createActorMaterial(source: Material, skin?: DualQuaternionSkin, linearSkinning = false): ShaderMaterial {
  const original = source as MeshStandardMaterial
  const region = !skin ? 6 : source.name.includes('brows') ? 5 : ({ skin: 1, hair: 2, linen: 3, eyes: 4, textured: 6, foliage: 7 } as Record<string, number>)[source.userData.region as string] ?? 2
  return new ShaderMaterial({
    name: `Pixel ${source.name}`,
    side: DoubleSide,
    defines: !skin ? { ACTOR_RIGID: 1 } : linearSkinning ? { ACTOR_LINEAR_SKINNING: 1 } : {},
    uniforms: {
      dqBones: { value: skin?.texture ?? null },
      baseColor: { value: (original.color?.clone() ?? new Color(.62, .30, .12)).convertLinearToSRGB() },
      lightDirection: { value: new Vector3(-3, 6, 4).normalize() },
      region: { value: region },
      colorMap: { value: original.map },
      useColorMap: { value: Boolean(original.map) },
    },
    vertexShader: `
      #include <common>
      #include <skinning_pars_vertex>
      #include <morphtarget_pars_vertex>
      #ifndef ACTOR_RIGID
        ${DQ_DECLARATIONS}
      #endif
      varying vec3 surfaceNormal;
      varying vec2 surfaceUV;
      varying float surfaceHeight;
      void main() {
        #ifdef ACTOR_LINEAR_SKINNING
          #include <skinbase_vertex>
        #elif !defined(ACTOR_RIGID)
          loadDQ();
        #endif
        #include <beginnormal_vertex>
        #include <morphinstance_vertex>
        #include <morphnormal_vertex>
        #ifdef ACTOR_LINEAR_SKINNING
          #include <skinnormal_vertex>
        #elif !defined(ACTOR_RIGID)
          objectNormal = mat3(bindMatrixInverse) * rotateDQ(mat3(bindMatrix) * objectNormal);
        #endif
        surfaceNormal = normalize(mat3(modelMatrix) * objectNormal);
        surfaceUV = uv;
        #include <begin_vertex>
        #include <morphtarget_vertex>
        #ifdef ACTOR_LINEAR_SKINNING
          #include <skinning_vertex>
        #elif !defined(ACTOR_RIGID)
          vec3 bound = (bindMatrix * vec4(transformed, 1.0)).xyz;
          transformed = (bindMatrixInverse * vec4(rotateDQ(bound) + translateDQ(), 1.0)).xyz;
        #endif
        surfaceHeight = (modelMatrix * vec4(transformed, 1.0)).y;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(transformed, 1.0);
      }`,
    fragmentShader: `
      uniform vec3 baseColor;
      uniform vec3 lightDirection;
      uniform float region;
      uniform sampler2D colorMap;
      uniform bool useColorMap;
      varying vec3 surfaceNormal;
      varying vec2 surfaceUV;
      varying float surfaceHeight;
      void main() {
        vec3 normal = normalize(surfaceNormal) * (gl_FrontFacing ? 1.0 : -1.0);
        float light = 0.48 + 0.86 * max(0.0, dot(normal, lightDirection));
        vec3 color = baseColor;
        if (useColorMap) color = pow(texture2D(colorMap, surfaceUV).rgb, vec3(1.0 / 2.2));
        float heightCode = floor(clamp(surfaceHeight / 2.4, 0.0, 1.0) * 31.0 + 0.5);
        gl_FragColor = vec4(clamp(color * light, 0.0, 1.0), (region * 32.0 + heightCode) / 255.0);
      }`,
  })
}
