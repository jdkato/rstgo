package rstgo

// settings mirrors the subset of docutils runtime settings that influence
// parsing, transforms and the HTML writers.
type settings struct {
	writer string // "html4" or "html5"

	// general
	title           string
	generator       bool
	datestamp       string // strftime-like layout already rendered, "" = none
	sourceLink      bool
	sourceURL       string
	tocBacklinks    string // "entry", "top", "" (none)
	footnoteBackl   bool
	sectnumXform    bool
	stripComments   bool
	stripClasses    []string
	stripElemsClass []string
	reportLevel     int
	haltLevel       int
	exitStatusLevel int
	languageCode    string
	idPrefix        string
	autoIDPrefix    string
	legacyIDs       bool

	// parser
	fileInsertionEnabled       bool
	rawEnabled                 bool
	lineLengthLimit            int
	pepReferences              bool
	pepBaseURL                 string
	pepFileURLTemplate         string
	rfcReferences              bool
	rfcBaseURL                 string
	tabWidth                   int
	trimFootnoteReferenceSpace *bool
	syntaxHighlight            string
	smartQuotes                string
	characterLevelInlineMarkup bool
	rootPrefix                 string

	// reader
	doctitleXform     bool
	docinfoXform      bool
	sectsubtitleXform bool

	// writer
	stylesheets         []string // references (link) or paths (embed)
	stylesheetIsPath    bool
	embedStylesheet     bool
	stylesheetDirs      []string
	initialHeaderLevel  int
	footnoteReferences  string // "superscript" or "brackets"
	attribution         string // "dash", "parentheses", "none"
	compactLists        bool
	compactFieldLists   bool
	tableStyle          string
	mathOutput          string
	xmlDeclaration      bool
	cloakEmailAddresses bool
	fieldNameLimit      int
	optionLimit         int
	sectionSelfLink     bool
	imageLoading        string
	outputEncoding      string
	sourcePath          string // path used for source link / includes ("" for stdin)
	destPath            string
	hasSource           bool
	viewport            string
	inputLen            int // size of the input, to size output buffers
}

func (st *settings) trimFootnoteRefSpace() bool {
	if st.trimFootnoteReferenceSpace == nil {
		return st.footnoteReferences == "superscript"
	}
	return *st.trimFootnoteReferenceSpace
}
