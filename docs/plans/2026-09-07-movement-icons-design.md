# Movement icons: approved variant A

The user approved the carved-silhouette direction on 2026-09-07.

Replace `web_new/public/assets/img/movement_modes.png` with four minimal ivory RPG figures: Crawl, Walk, Run, and Fast Run, facing right. Preserve the approved tunic, boots, angular carved edges, and distinct movement poses. The sprint includes restrained speed cuts.

Keep the existing 165 × 53 transparent atlas and movement-mode click coordinates. Fit every silhouette entirely within its existing tint region, with transparent gutters and a shared baseline. Keep the current gold selected-state tint and behavior.

Production artwork was generated with the built-in image generation tool from approved variant A. The final generation prompt requested only the four main-row figures as a white-on-black opacity mask, with no labels, secondary row, texture, shading, or framing. Convert mask luminance to alpha, apply a uniform warm ivory color, and downsample while preserving proportions.

Implementation: package the generated mask into the existing atlas, inspect the result at native and enlarged sizes, verify alpha and tint-zone isolation, and run the web client build. No interaction changes are needed.
