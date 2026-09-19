export namespace binaries {
	
	export class Problem {
	    name: string;
	    message: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new Problem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	    }
	}
	export class Status {
	    ready: boolean;
	    binDir: string;
	    firstRun: boolean;
	    versions: Record<string, string>;
	    problems: Problem[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.binDir = source["binDir"];
	        this.firstRun = source["firstRun"];
	        this.versions = source["versions"];
	        this.problems = this.convertValues(source["problems"], Problem);
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
	export class UpdateResult {
	    versionBefore: string;
	    versionAfter: string;
	    updated: boolean;
	    output: string;
	    fixups: string[];
	
	    static createFrom(source: any = {}) {
	        return new UpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionBefore = source["versionBefore"];
	        this.versionAfter = source["versionAfter"];
	        this.updated = source["updated"];
	        this.output = source["output"];
	        this.fixups = source["fixups"];
	    }
	}

}

export namespace core {
	
	export class Enhancements {
	    sponsorBlock: string;
	    sponsorBlockCategories: string[];
	    embedChapters: boolean;
	    embedThumbnail: boolean;
	    embedMetadata: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Enhancements(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sponsorBlock = source["sponsorBlock"];
	        this.sponsorBlockCategories = source["sponsorBlockCategories"];
	        this.embedChapters = source["embedChapters"];
	        this.embedThumbnail = source["embedThumbnail"];
	        this.embedMetadata = source["embedMetadata"];
	    }
	}
	export class Thumbnail {
	    url: string;
	    width: number;
	    height: number;
	    preference: number;
	
	    static createFrom(source: any = {}) {
	        return new Thumbnail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.preference = source["preference"];
	    }
	}
	export class Entry {
	    id: string;
	    title: string;
	    url: string;
	    duration: number;
	    uploader: string;
	    thumbnails: Thumbnail[];
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.duration = source["duration"];
	        this.uploader = source["uploader"];
	        this.thumbnails = this.convertValues(source["thumbnails"], Thumbnail);
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
	export class Format {
	    id: string;
	    ext: string;
	    note: string;
	    width: number;
	    height: number;
	    fps: number;
	    vcodec: string;
	    acodec: string;
	    filesize: number;
	    filesizeApprox: number;
	    tbr: number;
	    dynamicRange: string;
	
	    static createFrom(source: any = {}) {
	        return new Format(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ext = source["ext"];
	        this.note = source["note"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.vcodec = source["vcodec"];
	        this.acodec = source["acodec"];
	        this.filesize = source["filesize"];
	        this.filesizeApprox = source["filesizeApprox"];
	        this.tbr = source["tbr"];
	        this.dynamicRange = source["dynamicRange"];
	    }
	}
	export class Progress {
	    stage: string;
	    percent: number;
	    downloaded: number;
	    total: number;
	    speed: number;
	    eta: number;
	    fragment: number;
	    fragments: number;
	    item: number;
	    items: number;
	    detail: string;
	    filename: string;
	
	    static createFrom(source: any = {}) {
	        return new Progress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stage = source["stage"];
	        this.percent = source["percent"];
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.speed = source["speed"];
	        this.eta = source["eta"];
	        this.fragment = source["fragment"];
	        this.fragments = source["fragments"];
	        this.item = source["item"];
	        this.items = source["items"];
	        this.detail = source["detail"];
	        this.filename = source["filename"];
	    }
	}
	export class Output {
	    folder: string;
	    template: string;
	
	    static createFrom(source: any = {}) {
	        return new Output(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.template = source["template"];
	    }
	}
	export class Network {
	    rateLimit: string;
	    cookies: string;
	
	    static createFrom(source: any = {}) {
	        return new Network(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rateLimit = source["rateLimit"];
	        this.cookies = source["cookies"];
	    }
	}
	export class Music {
	    tags: boolean;
	    splitChapters: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Music(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tags = source["tags"];
	        this.splitChapters = source["splitChapters"];
	    }
	}
	export class Playlist {
	    start: number;
	    end: number;
	    reverse: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Playlist(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.reverse = source["reverse"];
	    }
	}
	export class Subtitles {
	    download: boolean;
	    embed: boolean;
	    languages: string[];
	    autoGenerated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Subtitles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.download = source["download"];
	        this.embed = source["embed"];
	        this.languages = source["languages"];
	        this.autoGenerated = source["autoGenerated"];
	    }
	}
	export class Options {
	    url: string;
	    pick: string;
	    container: string;
	    videoCodec: string;
	    audioCodec: string;
	    preferHDR: boolean;
	    subtitles: Subtitles;
	    enhancements: Enhancements;
	    playlist: Playlist;
	    music: Music;
	    network: Network;
	    output: Output;
	    ffmpegLocation: string;
	    arcProfileDir: string;
	    denoPath: string;
	
	    static createFrom(source: any = {}) {
	        return new Options(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.pick = source["pick"];
	        this.container = source["container"];
	        this.videoCodec = source["videoCodec"];
	        this.audioCodec = source["audioCodec"];
	        this.preferHDR = source["preferHDR"];
	        this.subtitles = this.convertValues(source["subtitles"], Subtitles);
	        this.enhancements = this.convertValues(source["enhancements"], Enhancements);
	        this.playlist = this.convertValues(source["playlist"], Playlist);
	        this.music = this.convertValues(source["music"], Music);
	        this.network = this.convertValues(source["network"], Network);
	        this.output = this.convertValues(source["output"], Output);
	        this.ffmpegLocation = source["ffmpegLocation"];
	        this.arcProfileDir = source["arcProfileDir"];
	        this.denoPath = source["denoPath"];
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
	export class Item {
	    id: string;
	    options: Options;
	    title: string;
	    state: string;
	    progress: Progress;
	    addedAt: number;
	    message: string;
	    detail: string;
	    errorKind: string;
	    notice: string;
	    filePath: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.options = this.convertValues(source["options"], Options);
	        this.title = source["title"];
	        this.state = source["state"];
	        this.progress = this.convertValues(source["progress"], Progress);
	        this.addedAt = source["addedAt"];
	        this.message = source["message"];
	        this.detail = source["detail"];
	        this.errorKind = source["errorKind"];
	        this.notice = source["notice"];
	        this.filePath = source["filePath"];
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
	export class ResolutionTier {
	    height: number;
	    label: string;
	    detail: string;
	    hasHighFrameRate: boolean;
	    hasHDR: boolean;
	    hdrFormat: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ResolutionTier(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.height = source["height"];
	        this.label = source["label"];
	        this.detail = source["detail"];
	        this.hasHighFrameRate = source["hasHighFrameRate"];
	        this.hasHDR = source["hasHDR"];
	        this.hdrFormat = source["hdrFormat"];
	        this.bytes = source["bytes"];
	    }
	}
	export class QualityOptions {
	    tiers: ResolutionTier[];
	    hasVideo: boolean;
	    hasAudio: boolean;
	    bestHeight: number;
	    bestLabel: string;
	    approximate: boolean;
	    countedFormats: number;
	    audioBytes: number;
	    bestBytes: number;
	    losslessAudio: boolean;
	    limited: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QualityOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tiers = this.convertValues(source["tiers"], ResolutionTier);
	        this.hasVideo = source["hasVideo"];
	        this.hasAudio = source["hasAudio"];
	        this.bestHeight = source["bestHeight"];
	        this.bestLabel = source["bestLabel"];
	        this.approximate = source["approximate"];
	        this.countedFormats = source["countedFormats"];
	        this.audioBytes = source["audioBytes"];
	        this.bestBytes = source["bestBytes"];
	        this.losslessAudio = source["losslessAudio"];
	        this.limited = source["limited"];
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
	export class Metadata {
	    kind: string;
	    id: string;
	    title: string;
	    uploader: string;
	    webpageUrl: string;
	    duration: number;
	    thumbnails: Thumbnail[];
	    formats: Format[];
	    entries: Entry[];
	    quality: QualityOptions;
	
	    static createFrom(source: any = {}) {
	        return new Metadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.uploader = source["uploader"];
	        this.webpageUrl = source["webpageUrl"];
	        this.duration = source["duration"];
	        this.thumbnails = this.convertValues(source["thumbnails"], Thumbnail);
	        this.formats = this.convertValues(source["formats"], Format);
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.quality = this.convertValues(source["quality"], QualityOptions);
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

export namespace doctor {
	
	export class Check {
	    id: string;
	    title: string;
	    status: string;
	    summary: string;
	    remedy: string;
	    detail: string;
	    fixable: boolean;
	    fixLabel: string;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.summary = source["summary"];
	        this.remedy = source["remedy"];
	        this.detail = source["detail"];
	        this.fixable = source["fixable"];
	        this.fixLabel = source["fixLabel"];
	    }
	}
	export class Report {
	    checks: Check[];
	    healthy: boolean;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new Report(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checks = this.convertValues(source["checks"], Check);
	        this.healthy = source["healthy"];
	        this.summary = source["summary"];
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

export namespace history {
	
	export class Entry {
	    id: string;
	    url: string;
	    title: string;
	    options: core.Options;
	    filePath: string;
	    state: string;
	    message: string;
	    errorKind: string;
	    notice: string;
	    bytes: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.title = source["title"];
	        this.options = this.convertValues(source["options"], core.Options);
	        this.filePath = source["filePath"];
	        this.state = source["state"];
	        this.message = source["message"];
	        this.errorKind = source["errorKind"];
	        this.notice = source["notice"];
	        this.bytes = source["bytes"];
	        this.finishedAt = source["finishedAt"];
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
	
	export class EventNames {
	    queueItem: string;
	    queueProgress: string;
	    binaryStatus: string;
	    settingsChanged: string;
	    queueRemoved: string;
	    historyChanged: string;
	
	    static createFrom(source: any = {}) {
	        return new EventNames(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.queueItem = source["queueItem"];
	        this.queueProgress = source["queueProgress"];
	        this.binaryStatus = source["binaryStatus"];
	        this.settingsChanged = source["settingsChanged"];
	        this.queueRemoved = source["queueRemoved"];
	        this.historyChanged = source["historyChanged"];
	    }
	}
	export class Settings {
	    downloadFolder: string;
	    concurrency: number;
	    cookies: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloadFolder = source["downloadFolder"];
	        this.concurrency = source["concurrency"];
	        this.cookies = source["cookies"];
	    }
	}

}

export namespace presets {
	
	export class Preset {
	    id: string;
	    name: string;
	    options: core.Options;
	    builtIn: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.options = this.convertValues(source["options"], core.Options);
	        this.builtIn = source["builtIn"];
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

export namespace updater {
	
	export class Result {
	    version: string;
	    installed: boolean;
	    needsRestart: boolean;
	    output: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.installed = source["installed"];
	        this.needsRestart = source["needsRestart"];
	        this.output = source["output"];
	    }
	}
	export class Update {
	    version: string;
	    notes: string;
	    pageUrl: string;
	    available: boolean;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Update(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.notes = source["notes"];
	        this.pageUrl = source["pageUrl"];
	        this.available = source["available"];
	        this.bytes = source["bytes"];
	    }
	}

}

