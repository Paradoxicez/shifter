// Leaflet + react-leaflet-cluster require three CSS files to be imported AT
// MODULE-LOAD TIME for tiles + popups + cluster icons to render correctly.
//
// Pitfall #4 (RESEARCH): without these imports, the map shows as a grey box
// and popups appear unstyled. Import THIS module once at the app entrypoint
// (web/src/App.tsx) — every component that uses Leaflet relies on this side
// effect.

import 'leaflet/dist/leaflet.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.Default.css'
