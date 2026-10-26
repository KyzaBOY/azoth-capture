export namespace aimarks {
	
	export class Draft {
	    id: number;
	    t_ms: number;
	    text: string;
	    kind: string;
	    screen_text: string;
	    candidates: string[];
	    confidence: number;
	    unsure: boolean;
	    frame: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Draft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.t_ms = source["t_ms"];
	        this.text = source["text"];
	        this.kind = source["kind"];
	        this.screen_text = source["screen_text"];
	        this.candidates = source["candidates"];
	        this.confidence = source["confidence"];
	        this.unsure = source["unsure"];
	        this.frame = source["frame"];
	        this.status = source["status"];
	    }
	}
	export class HitRate {
	    model: string;
	    accepted: number;
	    rejected: number;
	
	    static createFrom(source: any = {}) {
	        return new HitRate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.accepted = source["accepted"];
	        this.rejected = source["rejected"];
	    }
	}
	export class ModelInfo {
	    id: string;
	    efforts: string[];
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.efforts = source["efforts"];
	    }
	}
	export class Result {
	    model: string;
	    started_ms: number;
	    finished_ms: number;
	    frames: number;
	    parts: number;
	    failed: number;
	    notes: string[];
	    drafts: Draft[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.started_ms = source["started_ms"];
	        this.finished_ms = source["finished_ms"];
	        this.frames = source["frames"];
	        this.parts = source["parts"];
	        this.failed = source["failed"];
	        this.notes = source["notes"];
	        this.drafts = this.convertValues(source["drafts"], Draft);
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

export namespace config {
	
	export class AISettings {
	    enabled: boolean;
	    connector: string;
	    base_url: string;
	    api_key: string;
	    cli: string;
	    model: string;
	    effort: string;
	    data_source: string;
	    data_dir: string;
	    sample_every: number;
	
	    static createFrom(source: any = {}) {
	        return new AISettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.connector = source["connector"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.cli = source["cli"];
	        this.model = source["model"];
	        this.effort = source["effort"];
	        this.data_source = source["data_source"];
	        this.data_dir = source["data_dir"];
	        this.sample_every = source["sample_every"];
	    }
	}
	export class Settings {
	    hotkey: string;
	    always_on_top: boolean;
	    output_dir: string;
	    exe_path: string;
	    timeout: number;
	    one_per_endpoint: boolean;
	    video_allowed: boolean;
	    video_target: string;
	    video_audio: string;
	    video_hotkey: string;
	    video_codec: string;
	    video_quality: string;
	    ai: AISettings;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.always_on_top = source["always_on_top"];
	        this.output_dir = source["output_dir"];
	        this.exe_path = source["exe_path"];
	        this.timeout = source["timeout"];
	        this.one_per_endpoint = source["one_per_endpoint"];
	        this.video_allowed = source["video_allowed"];
	        this.video_target = source["video_target"];
	        this.video_audio = source["video_audio"];
	        this.video_hotkey = source["video_hotkey"];
	        this.video_codec = source["video_codec"];
	        this.video_quality = source["video_quality"];
	        this.ai = this.convertValues(source["ai"], AISettings);
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

export namespace journal {
	
	export class DraftInfo {
	    found: boolean;
	    name: string;
	    activity: string;
	    phase: string;
	    steps: number;
	    started_ms: number;
	    saved_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new DraftInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.name = source["name"];
	        this.activity = source["activity"];
	        this.phase = source["phase"];
	        this.steps = source["steps"];
	        this.started_ms = source["started_ms"];
	        this.saved_ms = source["saved_ms"];
	    }
	}
	export class Part {
	    number: number;
	    zip: string;
	    first_ms: number;
	    last_ms: number;
	    steps: number;
	    bytes: number;
	    problem: string;
	
	    static createFrom(source: any = {}) {
	        return new Part(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.number = source["number"];
	        this.zip = source["zip"];
	        this.first_ms = source["first_ms"];
	        this.last_ms = source["last_ms"];
	        this.steps = source["steps"];
	        this.bytes = source["bytes"];
	        this.problem = source["problem"];
	    }
	}
	export class SplitResult {
	    dir: string;
	    parts: Part[];
	
	    static createFrom(source: any = {}) {
	        return new SplitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.parts = this.convertValues(source["parts"], Part);
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
	export class Step {
	    id: number;
	    t_ms: number;
	    text: string;
	    target?: number;
	    via: string;
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.t_ms = source["t_ms"];
	        this.text = source["text"];
	        this.target = source["target"];
	        this.via = source["via"];
	    }
	}
	export class State {
	    schema: number;
	    phase: string;
	    name: string;
	    activity: string;
	    started_ms: number;
	    stopped_ms: number;
	    targets: number[];
	    steps: Step[];
	    next_id: number;
	    exported: boolean;
	    saved_ms: number;
	    export_dir: string;
	    draft_error: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.phase = source["phase"];
	        this.name = source["name"];
	        this.activity = source["activity"];
	        this.started_ms = source["started_ms"];
	        this.stopped_ms = source["stopped_ms"];
	        this.targets = source["targets"];
	        this.steps = this.convertValues(source["steps"], Step);
	        this.next_id = source["next_id"];
	        this.exported = source["exported"];
	        this.saved_ms = source["saved_ms"];
	        this.export_dir = source["export_dir"];
	        this.draft_error = source["draft_error"];
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

export namespace main {
	
	export class AIDecision {
	    id: number;
	    accept: boolean;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new AIDecision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accept = source["accept"];
	        this.text = source["text"];
	    }
	}
	export class AIStatus {
	    settings: config.AISettings;
	    has_key: boolean;
	    clis: string[];
	    running: boolean;
	    progress: string;
	    error: string;
	    result?: aimarks.Result;
	    stats: aimarks.HitRate[];
	
	    static createFrom(source: any = {}) {
	        return new AIStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], config.AISettings);
	        this.has_key = source["has_key"];
	        this.clis = source["clis"];
	        this.running = source["running"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.result = this.convertValues(source["result"], aimarks.Result);
	        this.stats = this.convertValues(source["stats"], aimarks.HitRate);
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
	export class VideoStatus {
	    info: video.Info;
	    allowed: boolean;
	    recording: boolean;
	    target: string;
	    window_picked: boolean;
	    audio: string;
	    hotkey: string;
	    hotkey_error: string;
	    clips: number;
	    error: string;
	    codec: string;
	    quality: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.info = this.convertValues(source["info"], video.Info);
	        this.allowed = source["allowed"];
	        this.recording = source["recording"];
	        this.target = source["target"];
	        this.window_picked = source["window_picked"];
	        this.audio = source["audio"];
	        this.hotkey = source["hotkey"];
	        this.hotkey_error = source["hotkey_error"];
	        this.clips = source["clips"];
	        this.error = source["error"];
	        this.codec = source["codec"];
	        this.quality = source["quality"];
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
	export class Boot {
	    version: string;
	    settings: config.Settings;
	    activities: string[];
	    draft: journal.DraftInfo;
	    state: journal.State;
	    hotkey_error: string;
	    env_ready: boolean;
	    app_dir: string;
	    platform: string;
	    video: VideoStatus;
	    ai: AIStatus;
	
	    static createFrom(source: any = {}) {
	        return new Boot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.settings = this.convertValues(source["settings"], config.Settings);
	        this.activities = source["activities"];
	        this.draft = this.convertValues(source["draft"], journal.DraftInfo);
	        this.state = this.convertValues(source["state"], journal.State);
	        this.hotkey_error = source["hotkey_error"];
	        this.env_ready = source["env_ready"];
	        this.app_dir = source["app_dir"];
	        this.platform = source["platform"];
	        this.video = this.convertValues(source["video"], VideoStatus);
	        this.ai = this.convertValues(source["ai"], AIStatus);
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
	export class ExportInfo {
	    dir: string;
	    state: journal.State;
	
	    static createFrom(source: any = {}) {
	        return new ExportInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.state = this.convertValues(source["state"], journal.State);
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
	export class HotkeyStatus {
	    hotkey: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HotkeyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hotkey = source["hotkey"];
	        this.error = source["error"];
	    }
	}
	export class NeededResult {
	    online: boolean;
	    error: string;
	    generated_at: string;
	    activities: needed.Activity[];
	
	    static createFrom(source: any = {}) {
	        return new NeededResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.online = source["online"];
	        this.error = source["error"];
	        this.generated_at = source["generated_at"];
	        this.activities = this.convertValues(source["activities"], needed.Activity);
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
	export class StopResult {
	    state: journal.State;
	    export_dir: string;
	    export_error: string;
	    send_dir: string;
	    size_warning: string;
	    video_clips: number;
	    video_error: string;
	
	    static createFrom(source: any = {}) {
	        return new StopResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = this.convertValues(source["state"], journal.State);
	        this.export_dir = source["export_dir"];
	        this.export_error = source["export_error"];
	        this.send_dir = source["send_dir"];
	        this.size_warning = source["size_warning"];
	        this.video_clips = source["video_clips"];
	        this.video_error = source["video_error"];
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

export namespace needed {
	
	export class Packet {
	    idx: number;
	    name: string;
	    short: string;
	    status: string;
	    what: string;
	    confidence: string;
	    confidence_label: string;
	    how_to_capture: string[];
	    notes: string;
	
	    static createFrom(source: any = {}) {
	        return new Packet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.idx = source["idx"];
	        this.name = source["name"];
	        this.short = source["short"];
	        this.status = source["status"];
	        this.what = source["what"];
	        this.confidence = source["confidence"];
	        this.confidence_label = source["confidence_label"];
	        this.how_to_capture = source["how_to_capture"];
	        this.notes = source["notes"];
	    }
	}
	export class Activity {
	    activity: string;
	    packets: Packet[];
	
	    static createFrom(source: any = {}) {
	        return new Activity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.activity = source["activity"];
	        this.packets = this.convertValues(source["packets"], Packet);
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

export namespace video {
	
	export class Monitor {
	    name: string;
	    size: string;
	
	    static createFrom(source: any = {}) {
	        return new Monitor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class Info {
	    available: boolean;
	    reason: string;
	    window_capture: boolean;
	    monitors: Monitor[];
	    needs_pick: boolean;
	    sound: boolean;
	    can_install: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.window_capture = source["window_capture"];
	        this.monitors = this.convertValues(source["monitors"], Monitor);
	        this.needs_pick = source["needs_pick"];
	        this.sound = source["sound"];
	        this.can_install = source["can_install"];
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

