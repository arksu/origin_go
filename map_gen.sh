rm -fr map_png

go run ./cmd/mapgen \
  -png-export -png-dir map_png -png-overview-only \
  -river-major-count 10 \
  -river-lake-count 280 \
  -river-lake-connect-chance 0.79 \
  -river-lake-connection-limit 260 \
  -river-lake-border-mix 0.45 \
  -river-max-lake-degree 3 \
  -river-width-min 5 \
  -river-width-max 14 \
  -river-shape-amplitude-scale 0.25 \
  -river-shape-frequency-scale 0.55 \
  -river-shape-noise-scale 0.24 \
  -river-shape-along-scale 0.12 \
  -river-shape-segment-length 56 \
  -river-shape-short-meander-scale 0.35 \
  -river-shape-short-meander-bias 0.005
