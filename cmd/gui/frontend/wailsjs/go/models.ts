export namespace history {
	
	export class BrowserDetection {
	    Name: string;
	    DBPath: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserDetection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.DBPath = source["DBPath"];
	    }
	}
	export class HistoryEntry {
	    id: number;
	    url: string;
	    title: string;
	    visitCount: number;
	    lastVisitTime: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.title = source["title"];
	        this.visitCount = source["visitCount"];
	        this.lastVisitTime = source["lastVisitTime"];
	    }
	}

}

export namespace main {
	
	export class BackupSnapshot {
	    path: string;
	    fileName: string;
	    createdUnix: number;
	    sizeBytes: number;
	    itemCount: number;
	
	    static createFrom(source: any = {}) {
	        return new BackupSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.fileName = source["fileName"];
	        this.createdUnix = source["createdUnix"];
	        this.sizeBytes = source["sizeBytes"];
	        this.itemCount = source["itemCount"];
	    }
	}
	export class BatchDeleteResult {
	    deleted: number;
	    backupPath: string;
	
	    static createFrom(source: any = {}) {
	        return new BatchDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deleted = source["deleted"];
	        this.backupPath = source["backupPath"];
	    }
	}
	export class BrowserEntry {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class BrowserProfileEntry {
	    name: string;
	    dbPath: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserProfileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dbPath = source["dbPath"];
	    }
	}
	export class BrowserWithProfiles {
	    name: string;
	    profiles: BrowserProfileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new BrowserWithProfiles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.profiles = this.convertValues(source["profiles"], BrowserProfileEntry);
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
	export class HistoryResult {
	    entries: history.HistoryEntry[];
	    total: number;
	    page: number;
	    pageSize: number;
	    totalPages: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], history.HistoryEntry);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.totalPages = source["totalPages"];
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
	export class SearchHistoryOptions {
	    dbPath: string;
	    keyword: string;
	    startDate: string;
	    endDate: string;
	    page: number;
	    pageSize: number;
	    sortBy: string;
	    sortOrder: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHistoryOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dbPath = source["dbPath"];
	        this.keyword = source["keyword"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.sortBy = source["sortBy"];
	        this.sortOrder = source["sortOrder"];
	    }
	}

}

