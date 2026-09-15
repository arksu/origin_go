# Stone axe — phase 2

Approved: one-handed travel grip, axe at the side, slightly bent elbow, restrained arm swing synchronized with walking. No attacks. Existing public equipment protocol remains authoritative.

- Adapt the supplied Meshy GLB to approximately 1,000–1,500 triangles, retain the painted silhouette/UVs, reduce texture cost and normalize scale/pivot at the grip. Keep the supplied original unchanged.
- Support left/right equipment slots independently using the phase-1 sockets. Author matching idle/walk arm clips on the existing commoner rig, preserving the base body geometry and locomotion.
- Select equipment arm clips from the catalog; sample walk clips by the same distance phase as locomotion. Carrying a large object retains priority and temporarily hides hand equipment.
- Verify attachment transforms, repeated gait samples, transition to idle, opposite-arm independence, both hands, unloading, server snapshots and eight game-camera directions. Check readability over dark green with upper-left static light.

Implementation order: inspect source/rig; reproducible asset preparation; arm clip export and catalog integration; regression tests and in-game WebGL review.
