package web

import "embed"

// Assets contains only production resources, so tests and docs are never served.
//
//go:embed index.html style.css app.mjs model.mjs feed.mjs fuel.mjs fuel-controls.mjs
var Assets embed.FS
