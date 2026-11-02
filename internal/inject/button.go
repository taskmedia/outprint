package inject

import "github.com/taskmedia/outprint/internal/button"

// ButtonConfig is how the script is told to place the PDF action through
// data attributes of its tag. The Gateway reads the same variables and sends
// them with the status; the attributes override that.
type ButtonConfig = button.Config
