#!/usr/bin/env python3
"""Roundtrip test: pack fixtures -> extract -> must be pixel-identical.

Also checks determinism (same input -> byte-identical atlas) and, with
--regression, re-extracts the real tiles atlas and compares it to
art_source/tiles.

Run: python3 tools/tests/roundtrip_test.py [--regression]
"""
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
TOOLS = os.path.dirname(HERE)
ROOT = os.path.dirname(TOOLS)
PACK = os.path.join(TOOLS, "pack_texturepacker_atlas.py")
EXTRACT = os.path.join(TOOLS, "extract_texturepacker_atlas.py")

sys.path.insert(0, TOOLS)


def md5(path):
    h = hashlib.md5()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 16), b""):
            h.update(chunk)
    return h.hexdigest()


def tree_md5(root_dir):
    items = []
    for base, _dirs, files in os.walk(root_dir):
        for name in sorted(files):
            if name.startswith("."):
                continue
            p = os.path.join(base, name)
            items.append((os.path.relpath(p, root_dir), md5(p)))
    return sorted(items)


def run(cmd):
    print("  $", " ".join(cmd))
    subprocess.run(cmd, check=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--regression", action="store_true",
                    help="also re-extract web_new tiles atlas and diff vs art_source/tiles")
    args = ap.parse_args()

    fixtures = os.path.join(tempfile.mkdtemp(prefix="atlas-fixtures-"))
    run(["python3", os.path.join(HERE, "gen_fixtures.py"), fixtures])
    src_files = tree_md5(fixtures)
    assert src_files, "no fixtures generated"

    tmp = tempfile.mkdtemp(prefix="atlas-roundtrip-")
    cases = [
        ["--algorithm", "maxrects"],
        ["--algorithm", "skyline"],
        ["--algorithm", "maxrects", "--no-rotate"],
        ["--algorithm", "skyline", "--no-rotate"],
        ["--algorithm", "maxrects", "--no-trim"],
        ["--algorithm", "maxrects", "--padding", "2", "--border-padding", "1"],
        ["--algorithm", "skyline", "--key-prefix", "tiles/", "--key-exclude", "keep"],
    ]
    try:
        for i, flags in enumerate(cases):
            out = os.path.join(tmp, f"case{i}")
            os.makedirs(out)
            atlas_json = os.path.join(out, "atlas.json")
            atlas_png = os.path.join(out, "atlas.png")
            run(["python3", PACK, fixtures, atlas_json, atlas_png] + flags)
            # determinism: pack again (same filenames, different dir),
            # must be byte-identical
            out2 = os.path.join(out, "rerun")
            os.makedirs(out2)
            atlas2 = os.path.join(out2, "atlas.json")
            png2 = os.path.join(out2, "atlas.png")
            run(["python3", PACK, fixtures, atlas2, png2] + flags)
            assert md5(atlas_json) == md5(atlas2) and md5(atlas_png) == md5(png2), \
                f"nondeterministic output for {flags}"

            # extract with the inverse key mapping
            extract_flags = ["--strip-prefix="]
            if "--key-prefix" in flags:
                extract_flags = ["--strip-prefix", "tiles/"]
            out_dir = os.path.join(out, "unpacked")
            run(["python3", EXTRACT, atlas_json, atlas_png, out_dir] + extract_flags)
            assert tree_md5(out_dir) == src_files, f"roundtrip mismatch for {flags}"

            # no aliases ever; full field set always
            data = json.load(open(atlas_json))
            assert "aliases" not in data, "aliases must not be emitted"
            for key, fr in data["frames"].items():
                for field in ("frame", "rotated", "trimmed", "spriteSourceSize", "sourceSize"):
                    assert field in fr, f"{key}: missing {field}"

            # key prefix applied exactly where requested
            if "--key-prefix" in flags:
                assert any(k.startswith("tiles/seq/") for k in data["frames"]), "prefix missing"
                assert any(k.startswith("keep/") for k in data["frames"]), "exclude failed"
            print(f"case {i} OK: {' '.join(flags)}")

        if args.regression:
            atlas = os.path.join(tmp, "tiles.json")
            sheet = os.path.join(tmp, "tiles.png")
            shutil.copy(os.path.join(ROOT, "web_new/public/assets/game/tiles.json"), atlas)
            shutil.copy(os.path.join(ROOT, "web_new/public/assets/game/tiles.png"), sheet)
            out_dir = os.path.join(tmp, "tiles_out")
            run(["python3", EXTRACT, atlas, sheet, out_dir])
            # art_source/tiles may also contain user-added sprites not yet in
            # the atlas — compare only what the extractor produces.
            golden = tree_md5(out_dir)
            reference = dict(tree_md5(os.path.join(ROOT, "art_source/tiles")))
            assert golden, "regression produced no files"
            for rel, digest in golden:
                assert reference.get(rel) == digest, f"regression mismatch: {rel}"
            print(f"regression OK: {len(golden)} extracted files match art_source/tiles")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    print("roundtrip_test: PASS")


if __name__ == "__main__":
    main()
