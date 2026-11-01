package devseed

import "strings"

// Everything below is invented. The names are fictional; the addresses use
// the reserved example.invalid domain.
const (
	BoundCollectionName   = "Bound"
	UnboundCollectionName = "Unbound"
	BoundDocumentTitle    = "Spring plant sale"
	UnboundDocumentTitle  = "Garden notes"

	// BodySentence is the first sentence of the bound Document; the browser
	// test looks for it in the PDF.
	BodySentence = "Thank you for your enquiry about the spring plant sale."
	// OrganisationName is the "name" of the example Configuration, which the
	// PDF shows in its letterhead.
	OrganisationName = "Example Garden Society"

	// BoundSubject is the subject in the Metadata Block of the bound Document.
	BoundSubject = "Spring plant sale"

	// InvalidDocumentTitle is a third Document in the Bound Collection whose
	// Metadata Block has mistakes, so that the error an author sees can be
	// shown in the documentation screenshots.
	InvalidDocumentTitle = "Autumn bulb order"

	ChartCaption   = "Germination over six weeks"
	DiagramCaption = "From seed to flower"

	chartFile   = "chart.png"
	diagramFile = "diagram.svg"
)

// BoundCollectionOverview is the Collection overview that binds the neutral
// Template and the example Configuration.
func BoundCollectionOverview() string {
	return strings.Join([]string{
		"Letters of the garden society. The block below is the Binding.",
		"",
		"```yaml",
		"binding:",
		"  template: neutral",
		"  configuration: example",
		"```",
		"",
	}, "\n")
}

// UnboundCollectionOverview has no Binding block.
func UnboundCollectionOverview() string {
	return "Notes without a Binding: documents here have no download button.\n"
}

// boundBody is the bound Document without its images, as it is created before
// the images are uploaded.
func boundBody() string {
	return strings.Join([]string{
		"```yaml",
		"type: letter",
		"subject: " + BoundSubject,
		"```",
		"",
		BodySentence + " The sale takes place on **Saturday, 14 March**,",
		"starting at *ten o'clock*.",
		"",
		"| Plant | Price |",
		"|-------|-------|",
		"| Basil | 2.50 |",
		"| Thyme | 3.00 |",
		"",
	}, "\n")
}

// boundFigures adds the two images to the bound Document, each alone in its
// paragraph, without alt text and so without a caption: the neutral letter
// cannot typeset a captioned figure (see docs/development.md). The size
// follows Outline's syntax.
func boundFigures(chartURL, diagramURL string) string {
	return boundBody() +
		"\n![](" + chartURL + " \" =240x\")\n" +
		"\n![](" + diagramURL + " \" =240x\")\n"
}

// invalidBody has a misspelled field name (subjekt) and a Document Type that
// the Template does not have (leter). The renderer reports both.
func invalidBody() string {
	return strings.Join([]string{
		"```yaml",
		"type: leter",
		"subjekt: Autumn bulb order",
		"```",
		"",
		"Draft of the autumn bulb order, not ready to download yet.",
		"",
	}, "\n")
}

func unboundBody() string {
	return "Watering plan for the greenhouse, written without a Binding.\n"
}

// unboundFigures adds the two images with a caption each. In Outline the
// caption of an image is its alt text; the browser test reads the exported
// Markdown of this Document to check that (docs/images.md).
func unboundFigures(chartURL, diagramURL string) string {
	return unboundBody() +
		"\n![" + ChartCaption + "](" + chartURL + " \" =240x\")\n" +
		"\n![" + DiagramCaption + "](" + diagramURL + " \" =240x\")\n"
}

// hasImages reports whether a stored Document already holds both images.
func hasImages(text string) bool {
	return strings.Count(text, "](/api/attachments.redirect?id=") >= 2
}
