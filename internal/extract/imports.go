package extract

// importsExtractor reads each source's go.mod files into imports edges between our repos. Stub until B3 M1b.
var importsExtractor = Extractor{Name: "imports", Version: 1, Run: func(*Ctx) error { return nil }}
