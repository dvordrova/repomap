// The report's canvas: the scene canvas (scene-canvas.jsx) draws every
// level of the system map; this entry registers it and what the page's
// other scripts share with it.
import ELK from 'elkjs/lib/elk.bundled.js';
import './card-content.jsx';
import {createSceneFlow} from './scene-canvas.jsx';
import '@xyflow/react/dist/style.css';
import './canvas.css';

// Legacy small repository diagrams share the same embedded ELK instance code.
window.ELK = ELK;
window.rmCreateFlow = createSceneFlow;
