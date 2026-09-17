# Equipment pose review

Open the equipment source in Blender with its character preview linked. Run via
the existing Blender MCP client using a small runner:

```python
import runpy
runpy.run_path('/absolute/repo/tools/blender/equipment_pose_review.py')['run'](
    '/absolute/repo/tools/blender/nettle_shirt_pose_review.json',
    '/absolute/new/output/directory')
```

```sh
python3 tools/blender/mcp_client.py /path/to/runner.py --timeout 180
```

The output directory must be new. Open `index.html` for side-by-side front,
rear and elevated views, and `report.json` for measurements per pose. Body is
red and equipment blue. Review all views, particularly shoulders, armpits,
elbows, cuffs and hem. Intentional neck/cuff openings are not failures.

For another garment copy the JSON profile and set the mesh names and actual
character actions/frame samples. The runner clones meshes and the rig into a
temporary scene, clears animation and bone transforms between cases, and
drives both meshes from the same rig. Source objects and files are not saved.

The six shirt cases cover rest, idle, opposite walk phases, raised arms idle
and raised arms walking. They are sampled checks, not exhaustive animation
coverage. Add frames or action cases when adding new motions; layered axe
poses need the same base/overlay composition as runtime, not an isolated axe
action.

The collision diagnostic tests garment vertices AND triangle centers against
the nearest body surface. Signed clearance below -0.003 units within 0.08
units of the surface is flagged. These tolerances are profile-specific.
Open meshes, normals and sparse samples limit this approximation: zero
candidates does not prove no intersections or correct coverage. Nonzero
candidates return `needs_review`; never bless a rig merely because the hem
does not rise. A visual review is required even when there are no candidates.
Pass `strict=True` to `run` to raise an assertion when any pose contains
candidates (the report is still written). Invalid or unnormalized skin
weights, nonexistent bones and influence-budget violations always fail.

This is a Blender-source test. Before releasing, also review the exported
equipment in the actual client to catch bind-matrix/export differences.
