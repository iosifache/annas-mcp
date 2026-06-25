package modes

type BookSearchParams struct {
	Query          string `json:"query" mcp:"Search query for books (e.g., title, author, topic)"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" mcp:"Optional HTTP timeout in seconds; defaults to the server timeout"`
}

type BookDownloadParams struct {
	BookHash       string `json:"hash" mcp:"MD5 hash of the book to download"`
	Title          string `json:"title" mcp:"Book title, used for filename"`
	Format         string `json:"format" mcp:"Book format, for example pdf or epub"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" mcp:"Optional HTTP timeout in seconds; defaults to the server timeout"`
}

type ArticleSearchParams struct {
	Query          string `json:"query" mcp:"DOI (e.g., '10.1038/nature12345') or search keywords for articles"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" mcp:"Optional HTTP timeout in seconds; defaults to the server timeout"`
}

type ArticleDownloadParams struct {
	DOI            string `json:"doi" mcp:"DOI of the article to download (e.g., '10.1038/nature12345')"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" mcp:"Optional HTTP timeout in seconds; defaults to the server timeout"`
}
