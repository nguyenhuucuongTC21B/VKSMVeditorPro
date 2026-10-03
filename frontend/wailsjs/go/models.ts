export namespace engine {
	
	export class ExportRequest {
	    format: string;
	    outputPath: string;
	    crf: number;
	    targetH: number;
	    preset: string;
	    gifFps: number;
	    hwAccel: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.outputPath = source["outputPath"];
	        this.crf = source["crf"];
	        this.targetH = source["targetH"];
	        this.preset = source["preset"];
	        this.gifFps = source["gifFps"];
	        this.hwAccel = source["hwAccel"];
	    }
	}

}

export namespace main {
	
	export class AISettingsResponse {
	    baseUrl: string;
	    model: string;
	    asrModel: string;
	    hasKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AISettingsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.asrModel = source["asrModel"];
	        this.hasKey = source["hasKey"];
	    }
	}
	export class AudioClipPatch {
	    startMs?: number;
	    inMs?: number;
	    outMs?: number;
	    volume?: number;
	    mute?: boolean;
	    fadeInMs?: number;
	    fadeOutMs?: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioClipPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startMs = source["startMs"];
	        this.inMs = source["inMs"];
	        this.outMs = source["outMs"];
	        this.volume = source["volume"];
	        this.mute = source["mute"];
	        this.fadeInMs = source["fadeInMs"];
	        this.fadeOutMs = source["fadeOutMs"];
	    }
	}
	export class ClipPatch {
	    inMs?: number;
	    outMs?: number;
	    mute?: boolean;
	    volume?: number;
	    rotateDeg?: number;
	    flipH?: boolean;
	    flipV?: boolean;
	    zoom?: number;
	    speed?: number;
	    reverse?: boolean;
	    boomerang?: boolean;
	    loopN?: number;
	    freezeStartMs?: number;
	    freezeEndMs?: number;
	    audioFadeInMs?: number;
	    audioFadeOutMs?: number;
	    waterDropMs?: number;
	    effects?: project.ClipEffects;
	    lutPath?: string;
	    keyframes?: project.KeyframePoint[];
	    opacity?: number;
	    posX?: number;
	    posY?: number;
	
	    static createFrom(source: any = {}) {
	        return new ClipPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inMs = source["inMs"];
	        this.outMs = source["outMs"];
	        this.mute = source["mute"];
	        this.volume = source["volume"];
	        this.rotateDeg = source["rotateDeg"];
	        this.flipH = source["flipH"];
	        this.flipV = source["flipV"];
	        this.zoom = source["zoom"];
	        this.speed = source["speed"];
	        this.reverse = source["reverse"];
	        this.boomerang = source["boomerang"];
	        this.loopN = source["loopN"];
	        this.freezeStartMs = source["freezeStartMs"];
	        this.freezeEndMs = source["freezeEndMs"];
	        this.audioFadeInMs = source["audioFadeInMs"];
	        this.audioFadeOutMs = source["audioFadeOutMs"];
	        this.waterDropMs = source["waterDropMs"];
	        this.effects = this.convertValues(source["effects"], project.ClipEffects);
	        this.lutPath = source["lutPath"];
	        this.keyframes = this.convertValues(source["keyframes"], project.KeyframePoint);
	        this.opacity = source["opacity"];
	        this.posX = source["posX"];
	        this.posY = source["posY"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiagnosticsResponse {
	    version: string;
	    os: string;
	    arch: string;
	    dataDir: string;
	    ffmpegPath: string;
	    ffmpegOk: boolean;
	    ffmpegSize: number;
	    ffprobePath: string;
	    ffprobeOk: boolean;
	    ffprobeSize: number;
	    ffVersion: string;
	    ffReady: boolean;
	    assets: number;
	    clips: number;
	    currentJob: string;
	    logPath: string;
	    logTail: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.dataDir = source["dataDir"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffmpegOk = source["ffmpegOk"];
	        this.ffmpegSize = source["ffmpegSize"];
	        this.ffprobePath = source["ffprobePath"];
	        this.ffprobeOk = source["ffprobeOk"];
	        this.ffprobeSize = source["ffprobeSize"];
	        this.ffVersion = source["ffVersion"];
	        this.ffReady = source["ffReady"];
	        this.assets = source["assets"];
	        this.clips = source["clips"];
	        this.currentJob = source["currentJob"];
	        this.logPath = source["logPath"];
	        this.logTail = source["logTail"];
	    }
	}
	export class OverlayPatch {
	    startMs?: number;
	    endMs?: number;
	    posX?: number;
	    posY?: number;
	    text?: string;
	    fontSize?: number;
	    colorHex?: string;
	    outline?: boolean;
	    bold?: boolean;
	    anim?: string;
	    animMs?: number;
	    shape?: string;
	    widthPct?: number;
	    heightPct?: number;
	    assetId?: string;
	    scalePct?: number;
	    volume?: number;
	    muted?: boolean;
	    chromaKey?: project.ChromaKeySettings;
	    blend?: string;
	    opacity?: number;
	    fadeInMs?: number;
	    fadeOutMs?: number;
	    filterPreset?: string;
	
	    static createFrom(source: any = {}) {
	        return new OverlayPatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	        this.posX = source["posX"];
	        this.posY = source["posY"];
	        this.text = source["text"];
	        this.fontSize = source["fontSize"];
	        this.colorHex = source["colorHex"];
	        this.outline = source["outline"];
	        this.bold = source["bold"];
	        this.anim = source["anim"];
	        this.animMs = source["animMs"];
	        this.shape = source["shape"];
	        this.widthPct = source["widthPct"];
	        this.heightPct = source["heightPct"];
	        this.assetId = source["assetId"];
	        this.scalePct = source["scalePct"];
	        this.volume = source["volume"];
	        this.muted = source["muted"];
	        this.chromaKey = this.convertValues(source["chromaKey"], project.ChromaKeySettings);
	        this.blend = source["blend"];
	        this.opacity = source["opacity"];
	        this.fadeInMs = source["fadeInMs"];
	        this.fadeOutMs = source["fadeOutMs"];
	        this.filterPreset = source["filterPreset"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StateResponse {
	    doc?: project.Document;
	    version: string;
	    dataDir: string;
	    previewUrl: string;
	    os: string;
	
	    static createFrom(source: any = {}) {
	        return new StateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doc = this.convertValues(source["doc"], project.Document);
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.previewUrl = source["previewUrl"];
	        this.os = source["os"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TransInfo {
	    type: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new TransInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	    }
	}

}

export namespace project {
	
	export class Rate {
	    num: number;
	    den: number;
	
	    static createFrom(source: any = {}) {
	        return new Rate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.num = source["num"];
	        this.den = source["den"];
	    }
	}
	export class Asset {
	    id: string;
	    path: string;
	    name: string;
	    kind: string;
	    durationMs: number;
	    width: number;
	    height: number;
	    rate: Rate;
	    hasAudio: boolean;
	    codec: string;
	    sizeBytes: number;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.durationMs = source["durationMs"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.rate = this.convertValues(source["rate"], Rate);
	        this.hasAudio = source["hasAudio"];
	        this.codec = source["codec"];
	        this.sizeBytes = source["sizeBytes"];
	        this.missing = source["missing"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AudioClip {
	    id: string;
	    assetId: string;
	    startMs: number;
	    inMs: number;
	    outMs: number;
	    volume: number;
	    mute: boolean;
	    fadeInMs: number;
	    fadeOutMs: number;
	
	    static createFrom(source: any = {}) {
	        return new AudioClip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.assetId = source["assetId"];
	        this.startMs = source["startMs"];
	        this.inMs = source["inMs"];
	        this.outMs = source["outMs"];
	        this.volume = source["volume"];
	        this.mute = source["mute"];
	        this.fadeInMs = source["fadeInMs"];
	        this.fadeOutMs = source["fadeOutMs"];
	    }
	}
	export class Canvas {
	    width: number;
	    height: number;
	    rate: Rate;
	
	    static createFrom(source: any = {}) {
	        return new Canvas(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.rate = this.convertValues(source["rate"], Rate);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChromaKeySettings {
	    enabled: boolean;
	    hex: string;
	    similarity: number;
	    blend: number;
	
	    static createFrom(source: any = {}) {
	        return new ChromaKeySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.hex = source["hex"];
	        this.similarity = source["similarity"];
	        this.blend = source["blend"];
	    }
	}
	export class KeyframePoint {
	    prop: string;
	    atMs: number;
	    val: number;
	
	    static createFrom(source: any = {}) {
	        return new KeyframePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prop = source["prop"];
	        this.atMs = source["atMs"];
	        this.val = source["val"];
	    }
	}
	export class ClipEffects {
	    brightness: number;
	    contrast: number;
	    saturation: number;
	    gamma: number;
	    blur: number;
	    sharpen: number;
	    vignette: number;
	    grayscale: boolean;
	    invert: boolean;
	    denoise: boolean;
	    audioDenoise: boolean;
	    vocalEnhance: boolean;
	    cbShadowR: number;
	    cbShadowG: number;
	    cbShadowB: number;
	    cbMidR: number;
	    cbMidG: number;
	    cbMidB: number;
	    cbHighR: number;
	    cbHighG: number;
	    cbHighB: number;
	
	    static createFrom(source: any = {}) {
	        return new ClipEffects(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.brightness = source["brightness"];
	        this.contrast = source["contrast"];
	        this.saturation = source["saturation"];
	        this.gamma = source["gamma"];
	        this.blur = source["blur"];
	        this.sharpen = source["sharpen"];
	        this.vignette = source["vignette"];
	        this.grayscale = source["grayscale"];
	        this.invert = source["invert"];
	        this.denoise = source["denoise"];
	        this.audioDenoise = source["audioDenoise"];
	        this.vocalEnhance = source["vocalEnhance"];
	        this.cbShadowR = source["cbShadowR"];
	        this.cbShadowG = source["cbShadowG"];
	        this.cbShadowB = source["cbShadowB"];
	        this.cbMidR = source["cbMidR"];
	        this.cbMidG = source["cbMidG"];
	        this.cbMidB = source["cbMidB"];
	        this.cbHighR = source["cbHighR"];
	        this.cbHighG = source["cbHighG"];
	        this.cbHighB = source["cbHighB"];
	    }
	}
	export class Transform {
	    rotateDeg: number;
	    flipH: boolean;
	    flipV: boolean;
	    zoom: number;
	    posX: number;
	    posY: number;
	
	    static createFrom(source: any = {}) {
	        return new Transform(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rotateDeg = source["rotateDeg"];
	        this.flipH = source["flipH"];
	        this.flipV = source["flipV"];
	        this.zoom = source["zoom"];
	        this.posX = source["posX"];
	        this.posY = source["posY"];
	    }
	}
	export class Clip {
	    id: string;
	    assetId: string;
	    inMs: number;
	    outMs: number;
	    mute: boolean;
	    volume: number;
	    transform: Transform;
	    effects: ClipEffects;
	    lutPath: string;
	    speed: number;
	    reverse: boolean;
	    boomerang: boolean;
	    loopN: number;
	    freezeStartMs: number;
	    freezeEndMs: number;
	    audioFadeInMs: number;
	    audioFadeOutMs: number;
	    waterDropMs: number;
	    keyframes: KeyframePoint[];
	    opacity: number;
	
	    static createFrom(source: any = {}) {
	        return new Clip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.assetId = source["assetId"];
	        this.inMs = source["inMs"];
	        this.outMs = source["outMs"];
	        this.mute = source["mute"];
	        this.volume = source["volume"];
	        this.transform = this.convertValues(source["transform"], Transform);
	        this.effects = this.convertValues(source["effects"], ClipEffects);
	        this.lutPath = source["lutPath"];
	        this.speed = source["speed"];
	        this.reverse = source["reverse"];
	        this.boomerang = source["boomerang"];
	        this.loopN = source["loopN"];
	        this.freezeStartMs = source["freezeStartMs"];
	        this.freezeEndMs = source["freezeEndMs"];
	        this.audioFadeInMs = source["audioFadeInMs"];
	        this.audioFadeOutMs = source["audioFadeOutMs"];
	        this.waterDropMs = source["waterDropMs"];
	        this.keyframes = this.convertValues(source["keyframes"], KeyframePoint);
	        this.opacity = source["opacity"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Credits {
	    lines: string[];
	    fontSize: number;
	    speedPxs: number;
	
	    static createFrom(source: any = {}) {
	        return new Credits(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lines = source["lines"];
	        this.fontSize = source["fontSize"];
	        this.speedPxs = source["speedPxs"];
	    }
	}
	export class Cue {
	    startMs: number;
	    endMs: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Cue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	        this.text = source["text"];
	    }
	}
	export class Timecode {
	    enabled: boolean;
	    fontSize: number;
	    format: string;
	    posX: number;
	    posY: number;
	
	    static createFrom(source: any = {}) {
	        return new Timecode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.fontSize = source["fontSize"];
	        this.format = source["format"];
	        this.posX = source["posX"];
	        this.posY = source["posY"];
	    }
	}
	export class Watermark {
	    assetId: string;
	    corner: string;
	    marginPx: number;
	    opacity: number;
	    scalePct: number;
	
	    static createFrom(source: any = {}) {
	        return new Watermark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.assetId = source["assetId"];
	        this.corner = source["corner"];
	        this.marginPx = source["marginPx"];
	        this.opacity = source["opacity"];
	        this.scalePct = source["scalePct"];
	    }
	}
	export class SubtitlesTrack {
	    sourceName: string;
	    cues: Cue[];
	    burn: boolean;
	    fontSize: number;
	    colorHex: string;
	
	    static createFrom(source: any = {}) {
	        return new SubtitlesTrack(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceName = source["sourceName"];
	        this.cues = this.convertValues(source["cues"], Cue);
	        this.burn = source["burn"];
	        this.fontSize = source["fontSize"];
	        this.colorHex = source["colorHex"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Overlay {
	    id: string;
	    kind: string;
	    startMs: number;
	    endMs: number;
	    posX: number;
	    posY: number;
	    text?: string;
	    fontSize?: number;
	    colorHex?: string;
	    outline?: boolean;
	    anim?: string;
	    animMs?: number;
	    bold?: boolean;
	    shape?: string;
	    widthPct?: number;
	    heightPct?: number;
	    assetId?: string;
	    scalePct?: number;
	    volume?: number;
	    muted?: boolean;
	    chromaKey?: ChromaKeySettings;
	    blend?: string;
	    opacity?: number;
	    fadeInMs?: number;
	    fadeOutMs?: number;
	    filterPreset?: string;
	
	    static createFrom(source: any = {}) {
	        return new Overlay(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	        this.posX = source["posX"];
	        this.posY = source["posY"];
	        this.text = source["text"];
	        this.fontSize = source["fontSize"];
	        this.colorHex = source["colorHex"];
	        this.outline = source["outline"];
	        this.anim = source["anim"];
	        this.animMs = source["animMs"];
	        this.bold = source["bold"];
	        this.shape = source["shape"];
	        this.widthPct = source["widthPct"];
	        this.heightPct = source["heightPct"];
	        this.assetId = source["assetId"];
	        this.scalePct = source["scalePct"];
	        this.volume = source["volume"];
	        this.muted = source["muted"];
	        this.chromaKey = this.convertValues(source["chromaKey"], ChromaKeySettings);
	        this.blend = source["blend"];
	        this.opacity = source["opacity"];
	        this.fadeInMs = source["fadeInMs"];
	        this.fadeOutMs = source["fadeOutMs"];
	        this.filterPreset = source["filterPreset"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Transition {
	    afterClipId: string;
	    type: string;
	    durationMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Transition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.afterClipId = source["afterClipId"];
	        this.type = source["type"];
	        this.durationMs = source["durationMs"];
	    }
	}
	export class Document {
	    version: number;
	    name: string;
	    canvas: Canvas;
	    assets: Asset[];
	    clips: Clip[];
	    transitions: Transition[];
	    overlays: Overlay[];
	    subs?: SubtitlesTrack;
	    watermark?: Watermark;
	    timecode?: Timecode;
	    credits?: Credits;
	    muteAll: boolean;
	    hideOverlays: boolean;
	    audioClips: AudioClip[];
	
	    static createFrom(source: any = {}) {
	        return new Document(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.name = source["name"];
	        this.canvas = this.convertValues(source["canvas"], Canvas);
	        this.assets = this.convertValues(source["assets"], Asset);
	        this.clips = this.convertValues(source["clips"], Clip);
	        this.transitions = this.convertValues(source["transitions"], Transition);
	        this.overlays = this.convertValues(source["overlays"], Overlay);
	        this.subs = this.convertValues(source["subs"], SubtitlesTrack);
	        this.watermark = this.convertValues(source["watermark"], Watermark);
	        this.timecode = this.convertValues(source["timecode"], Timecode);
	        this.credits = this.convertValues(source["credits"], Credits);
	        this.muteAll = source["muteAll"];
	        this.hideOverlays = source["hideOverlays"];
	        this.audioClips = this.convertValues(source["audioClips"], AudioClip);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	

}

export namespace tools {
	
	export class QCProblem {
	    level: string;
	    kind: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new QCProblem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.kind = source["kind"];
	        this.message = source["message"];
	    }
	}
	export class Silence {
	    startMs: number;
	    endMs: number;
	
	    static createFrom(source: any = {}) {
	        return new Silence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.startMs = source["startMs"];
	        this.endMs = source["endMs"];
	    }
	}
	export class QCReport {
	    file: string;
	    durationMs: number;
	    meanVolumeDb?: number;
	    maxVolumeDb?: number;
	    blackRanges?: Silence[];
	    problems?: QCProblem[];
	
	    static createFrom(source: any = {}) {
	        return new QCReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.durationMs = source["durationMs"];
	        this.meanVolumeDb = source["meanVolumeDb"];
	        this.maxVolumeDb = source["maxVolumeDb"];
	        this.blackRanges = this.convertValues(source["blackRanges"], Silence);
	        this.problems = this.convertValues(source["problems"], QCProblem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SceneMark {
	    atMs: number;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new SceneMark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.atMs = source["atMs"];
	        this.score = source["score"];
	    }
	}
	
	export class UpdateInfo {
	    currentVersion: string;
	    latestVersion?: string;
	    updateUrl?: string;
	    assetUrl?: string;
	    notes?: string;
	    hasUpdate: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.updateUrl = source["updateUrl"];
	        this.assetUrl = source["assetUrl"];
	        this.notes = source["notes"];
	        this.hasUpdate = source["hasUpdate"];
	        this.error = source["error"];
	    }
	}
	export class VoiceInfo {
	    name: string;
	    language: string;
	    gender: string;
	
	    static createFrom(source: any = {}) {
	        return new VoiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.language = source["language"];
	        this.gender = source["gender"];
	    }
	}

}

