import { DataTexture, FloatType, Matrix4, Quaternion, RGBAFormat, Skeleton, Vector3 } from 'three'

/** Keep the source rig's muscle volume instead of reverting to linear skinning. */
export class DualQuaternionSkin {
  readonly texture: DataTexture
  private readonly values: Float32Array
  private readonly transform = new Matrix4()
  private readonly rotation = new Quaternion()
  private readonly translation = new Vector3()
  private readonly scale = new Vector3()

  constructor(readonly skeleton: Skeleton) {
    this.values = new Float32Array(skeleton.bones.length * 8)
    this.texture = new DataTexture(this.values, 2, skeleton.bones.length, RGBAFormat, FloatType)
    this.texture.needsUpdate = true
  }

  update(): void {
    const { transform, rotation, translation, scale, values } = this
    for (let index = 0; index < this.skeleton.bones.length; index++) {
      transform.multiplyMatrices(this.skeleton.bones[index]!.matrixWorld, this.skeleton.boneInverses[index]!)
      transform.decompose(translation, rotation, scale)
      if (Math.max(Math.abs(scale.x - 1), Math.abs(scale.y - 1), Math.abs(scale.z - 1)) > .002) {
        throw new Error('Dual quaternion actor skeleton must have unit bone scale')
      }
      const offset = index * 8
      rotation.toArray(values, offset)
      const { x: tx, y: ty, z: tz } = translation
      const { x: qx, y: qy, z: qz, w: qw } = rotation
      values[offset + 4] = .5 * (tx * qw + ty * qz - tz * qy)
      values[offset + 5] = .5 * (-tx * qz + ty * qw + tz * qx)
      values[offset + 6] = .5 * (tx * qy - ty * qx + tz * qw)
      values[offset + 7] = -.5 * (tx * qx + ty * qy + tz * qz)
    }
    this.texture.needsUpdate = true
  }

  destroy(): void { this.texture.dispose() }
}

export const DQ_DECLARATIONS = `
uniform sampler2D dqBones;
vec4 dqReal, dqDual;
vec3 rotateDQ(vec3 point) {
  return point + 2.0 * cross(dqReal.xyz, cross(dqReal.xyz, point) + dqReal.w * point);
}
void loadDQ() {
  vec4 first = texelFetch(dqBones, ivec2(0, int(skinIndex.x)), 0);
  dqReal = vec4(0.0); dqDual = vec4(0.0);
  for (int influence = 0; influence < 4; influence++) {
    vec4 realPart = texelFetch(dqBones, ivec2(0, int(skinIndex[influence])), 0);
    vec4 dualPart = texelFetch(dqBones, ivec2(1, int(skinIndex[influence])), 0);
    float weight = skinWeight[influence] * (dot(first, realPart) < 0.0 ? -1.0 : 1.0);
    dqReal += realPart * weight; dqDual += dualPart * weight;
  }
  float magnitude = max(length(dqReal), 0.000001);
  dqReal /= magnitude; dqDual /= magnitude;
}
vec3 translateDQ() {
  return 2.0 * (dqReal.w * dqDual.xyz - dqDual.w * dqReal.xyz + cross(dqReal.xyz, dqDual.xyz));
}`
