rm -fr map_png

go run ./cmd/mapgen \
  -gen-config etc/mapgen/presets/hnh.yaml \
  -png-export -png-dir map_png -png-overview-only
