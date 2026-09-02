package task

import "github.com/nabrahma/lathe/internal/detect"

// Engine identifiers. A task names the engine that executes it; the engine
// registry resolves the name to an implementation.
const (
	EnginePDF    = "pdfcpu"
	EngineImage  = "image"
	EngineOCR    = "tesseract"
	EngineMedia  = "ffmpeg"
	EngineOffice = "libreoffice"
)

var (
	pdfIn      = []detect.Category{detect.CategoryPDF}
	imageIn    = []detect.Category{detect.CategoryImage}
	documentIn = []detect.Category{detect.CategoryDocument}
	videoIn    = []detect.Category{detect.CategoryVideo}
	audioIn    = []detect.Category{detect.CategoryVideo, detect.CategoryAudio}
)

// qualityChoices is the single control most compression tasks need. It is
// deliberately three plain words: DPI, colour space and downsampling
// thresholds live behind Advanced, where the people who want them will look.
var qualityChoices = []Choice{
	{Value: "low", Label: "Smaller file", Hint: "Best for email and upload limits"},
	{Value: "medium", Label: "Balanced", Hint: "Good quality at a much smaller size"},
	{Value: "high", Label: "Best quality", Hint: "Only removes what is safe to remove"},
}

// Catalog returns every task Lathe ships with, in home-screen order.
func Catalog() []Task {
	tasks := make([]Task, 0, 32)
	tasks = append(tasks, pdfTasks()...)
	tasks = append(tasks, imageTasks()...)
	tasks = append(tasks, textTasks()...)
	tasks = append(tasks, documentTasks()...)
	tasks = append(tasks, mediaTasks()...)
	return tasks
}

// Default returns the shipped registry. It panics on a malformed definition,
// which is correct: a bad task is a programming error caught by the tests, not
// a runtime condition.
func Default() *Registry {
	r, err := NewRegistry(Catalog()...)
	if err != nil {
		panic(err)
	}
	return r
}

func pdfTasks() []Task {
	return []Task{
		{
			ID: "pdf.compress", Name: "Compress PDF", Description: "Make a PDF smaller",
			Category: CategoryPDF, Icon: "compress", Verb: "Compress",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "quality", Label: "Quality", Type: OptionChoice, Default: "medium", Choices: qualityChoices},
				{ID: "imageDPI", Label: "Image resolution", Type: OptionRange, Default: 150,
					Min: 72, Max: 600, Step: 6, Advanced: true},
				{ID: "password", Label: "Password", Type: OptionPassword, Advanced: true,
					Placeholder: "Only if the PDF is protected"},
			},
		},
		{
			ID: "pdf.merge", Name: "Merge PDFs", Description: "Combine several PDFs into one",
			Category: CategoryPDF, Icon: "merge", Verb: "Merge",
			Accepts: pdfIn, MinInputs: 2, MaxInputs: 0,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "bookmarks", Label: "Add a bookmark per file", Type: OptionToggle, Default: true},
			},
		},
		{
			ID: "pdf.split", Name: "Split PDF", Description: "Break a PDF into separate files",
			Category: CategoryPDF, Icon: "split", Verb: "Split",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "mode", Label: "Split", Type: OptionChoice, Default: "pages", Choices: []Choice{
					{Value: "pages", Label: "Into single pages"},
					{Value: "every", Label: "Every N pages"},
					{Value: "range", Label: "By page range"},
				}},
				{ID: "span", Label: "Pages per file", Type: OptionRange, Default: 1, Min: 1, Max: 500, Step: 1},
				{ID: "pages", Label: "Page range", Type: OptionPageRange, Default: "", Placeholder: "e.g. 1-3, 8, 11-"},
			},
		},
		{
			ID: "pdf.rotate", Name: "Rotate pages", Description: "Turn pages the right way up",
			Category: CategoryPDF, Icon: "rotate", Verb: "Rotate",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "angle", Label: "Turn by", Type: OptionChoice, Default: "90", Choices: []Choice{
					{Value: "90", Label: "90° right"},
					{Value: "180", Label: "180°"},
					{Value: "270", Label: "90° left"},
				}},
				{ID: "pages", Label: "Pages", Type: OptionPageRange, Default: "", Placeholder: "All pages"},
			},
		},
		{
			ID: "pdf.delete-pages", Name: "Delete pages", Description: "Remove pages from a PDF",
			Category: CategoryPDF, Icon: "delete", Verb: "Remove pages",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "pages", Label: "Pages to remove", Type: OptionPageRange, Default: "", Placeholder: "e.g. 2, 5-7"},
			},
		},
		{
			ID: "pdf.reorder", Name: "Reorder pages", Description: "Rearrange the pages of a PDF",
			Category: CategoryPDF, Icon: "reorder", Verb: "Apply order",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "order", Label: "Page order", Type: OptionText, Default: "", Placeholder: "e.g. 3, 1, 2"},
			},
		},
		{
			ID: "pdf.watermark", Name: "Add watermark", Description: "Stamp text across every page",
			Category: CategoryPDF, Icon: "watermark", Verb: "Add watermark",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "text", Label: "Text", Type: OptionText, Default: "DRAFT", Placeholder: "DRAFT"},
				{ID: "opacity", Label: "Strength", Type: OptionRange, Default: 0.3, Min: 0.05, Max: 1, Step: 0.05},
				{ID: "position", Label: "Position", Type: OptionChoice, Default: "center", Choices: []Choice{
					{Value: "center", Label: "Across the middle"},
					{Value: "diagonal", Label: "Diagonally"},
					{Value: "bottom", Label: "Along the bottom"},
				}},
			},
		},
		{
			ID: "pdf.protect", Name: "Protect PDF", Description: "Add a password to a PDF",
			Category: CategoryPDF, Icon: "lock", Verb: "Protect",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "password", Label: "Password", Type: OptionPassword, Default: "", Placeholder: "Choose a password"},
			},
		},
		{
			ID: "pdf.unlock", Name: "Unlock PDF", Description: "Remove a password you already know",
			Category: CategoryPDF, Icon: "unlock", Verb: "Unlock",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "password", Label: "Current password", Type: OptionPassword, Default: "",
					Placeholder: "The password that opens this file"},
			},
		},
		{
			ID: "pdf.to-images", Name: "PDF to images", Description: "Save each page as a picture",
			Category: CategoryPDF, Icon: "images", Verb: "Export pages",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "format", Label: "Save as", Type: OptionChoice, Default: "png", Choices: []Choice{
					{Value: "png", Label: "PNG", Hint: "Sharp, larger files"},
					{Value: "jpg", Label: "JPG", Hint: "Smaller files"},
				}},
				{ID: "pages", Label: "Pages", Type: OptionPageRange, Default: "", Placeholder: "All pages"},
			},
		},
		{
			ID: "pdf.from-images", Name: "Images to PDF", Description: "Combine pictures into one PDF",
			Category: CategoryPDF, Icon: "pdf", Verb: "Make PDF",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EnginePDF, RequiredTier: TierCore,
			Options: []Option{
				{ID: "pageSize", Label: "Page size", Type: OptionChoice, Default: "A4", Choices: []Choice{
					{Value: "A4", Label: "A4"},
					{Value: "Letter", Label: "Letter"},
					{Value: "fit", Label: "Fit each image"},
				}},
				{ID: "orientation", Label: "Orientation", Type: OptionChoice, Default: "auto", Choices: []Choice{
					{Value: "auto", Label: "Match the image"},
					{Value: "portrait", Label: "Portrait"},
					{Value: "landscape", Label: "Landscape"},
				}},
			},
		},
	}
}

func imageTasks() []Task {
	return []Task{
		{
			ID: "image.convert", Name: "Convert image", Description: "Change a picture to another format",
			Category: CategoryImage, Icon: "convert", Verb: "Convert",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineImage, RequiredTier: TierCore,
			Options: []Option{
				{ID: "format", Label: "Convert to", Type: OptionChoice, Default: "jpg", Choices: []Choice{
					{Value: "jpg", Label: "JPG", Hint: "Works everywhere"},
					{Value: "png", Label: "PNG", Hint: "Keeps transparency"},
					{Value: "webp", Label: "WEBP", Hint: "Smaller, modern"},
					{Value: "tiff", Label: "TIFF", Hint: "For print and scanning"},
				}},
				{ID: "quality", Label: "Quality", Type: OptionRange, Default: 85, Min: 40, Max: 100, Step: 5},
			},
		},
		{
			ID: "image.compress", Name: "Compress image", Description: "Make a picture smaller",
			Category: CategoryImage, Icon: "compress", Verb: "Compress",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineImage, RequiredTier: TierCore,
			Options: []Option{
				{ID: "quality", Label: "Quality", Type: OptionRange, Default: 75, Min: 30, Max: 100, Step: 5},
				{ID: "maxWidth", Label: "Limit width to", Type: OptionRange, Default: 0,
					Min: 0, Max: 8000, Step: 100, Advanced: true},
			},
		},
		{
			ID: "image.resize", Name: "Resize image", Description: "Change a picture's dimensions",
			Category: CategoryImage, Icon: "resize", Verb: "Resize",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineImage, RequiredTier: TierCore,
			Options: []Option{
				{ID: "preset", Label: "Size", Type: OptionChoice, Default: "custom", Choices: []Choice{
					{Value: "custom", Label: "Custom"},
					{Value: "passport", Label: "Passport photo", Hint: "413 × 531"},
					{Value: "profile", Label: "Profile picture", Hint: "512 × 512"},
					{Value: "hd", Label: "1920 wide"},
				}},
				{ID: "width", Label: "Width", Type: OptionRange, Default: 1024, Min: 16, Max: 12000, Step: 16},
				{ID: "height", Label: "Height", Type: OptionRange, Default: 0, Min: 0, Max: 12000, Step: 16},
				{ID: "keepAspect", Label: "Keep proportions", Type: OptionToggle, Default: true, Advanced: true},
			},
		},
		{
			ID: "image.crop", Name: "Crop image", Description: "Trim a picture to a shape",
			Category: CategoryImage, Icon: "crop", Verb: "Crop",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 1,
			Engine: EngineImage, RequiredTier: TierCore,
			Options: []Option{
				{ID: "aspect", Label: "Shape", Type: OptionChoice, Default: "free", Choices: []Choice{
					{Value: "free", Label: "Free"},
					{Value: "1:1", Label: "Square"},
					{Value: "4:3", Label: "4:3"},
					{Value: "16:9", Label: "16:9"},
				}},
				{ID: "rect", Label: "Area", Type: OptionText, Default: "", Placeholder: "x,y,width,height", Advanced: true},
			},
		},
	}
}

func textTasks() []Task {
	return []Task{
		{
			ID: "text.from-image", Name: "Extract text from image", Description: "Read the words in a picture",
			Category: CategoryText, Icon: "text", Verb: "Extract text",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineOCR, RequiredTier: TierBundled,
			Options: []Option{
				{ID: "lang", Label: "Language", Type: OptionText, Default: "eng", Placeholder: "eng"},
				{ID: "enhance", Label: "Enhance the image first", Type: OptionToggle, Default: true},
				{ID: "psm", Label: "Page layout", Type: OptionChoice, Default: "auto", Advanced: true, Choices: []Choice{
					{Value: "auto", Label: "Detect automatically"},
					{Value: "block", Label: "One block of text"},
					{Value: "line", Label: "A single line"},
					{Value: "word", Label: "A single word"},
					{Value: "sparse", Label: "Scattered text"},
				}},
			},
		},
		{
			ID: "text.from-pdf", Name: "Extract text from PDF", Description: "Pull the words out of a PDF",
			Category: CategoryText, Icon: "text", Verb: "Extract text",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EngineOCR, RequiredTier: TierCore,
			Options: []Option{
				{ID: "ocr", Label: "Read scanned pages too", Type: OptionToggle, Default: true},
				{ID: "lang", Label: "Language", Type: OptionText, Default: "eng", Placeholder: "eng"},
				{ID: "pages", Label: "Pages", Type: OptionPageRange, Default: "", Placeholder: "All pages"},
			},
		},
		{
			ID: "text.searchable-pdf", Name: "Make PDF searchable", Description: "Add selectable text to a scan",
			Category: CategoryText, Icon: "search", Verb: "Make searchable",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EngineOCR, RequiredTier: TierBundled,
			Options: []Option{
				{ID: "lang", Label: "Language", Type: OptionText, Default: "eng", Placeholder: "eng"},
				{ID: "enhance", Label: "Enhance pages first", Type: OptionToggle, Default: true},
			},
		},
		{
			ID: "text.image-to-document", Name: "Image to document", Description: "Read a picture into a Word file",
			Category: CategoryText, Icon: "document", Verb: "Create document",
			Accepts: imageIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineOCR, RequiredTier: TierOffice,
			Options: []Option{
				{ID: "format", Label: "Save as", Type: OptionChoice, Default: "docx", Choices: []Choice{
					{Value: "docx", Label: "Word (.docx)"},
					{Value: "pdf", Label: "PDF"},
					{Value: "txt", Label: "Plain text"},
				}},
				{ID: "lang", Label: "Language", Type: OptionText, Default: "eng", Placeholder: "eng"},
			},
		},
	}
}

func documentTasks() []Task {
	return []Task{
		{
			ID: "document.pdf-to-word", Name: "PDF to Word", Description: "Turn a PDF into an editable document",
			Category: CategoryDocument, Icon: "word", Verb: "Convert",
			Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
			Engine: EngineOffice, RequiredTier: TierOffice,
			Options: []Option{
				{ID: "format", Label: "Save as", Type: OptionChoice, Default: "docx", Choices: []Choice{
					{Value: "docx", Label: "Word (.docx)"},
					{Value: "odt", Label: "OpenDocument (.odt)"},
					{Value: "rtf", Label: "Rich text (.rtf)"},
				}},
			},
		},
		{
			ID: "document.to-pdf", Name: "Document to PDF", Description: "Convert Word, Excel or PowerPoint to PDF",
			Category: CategoryDocument, Icon: "pdf", Verb: "Convert",
			Accepts: documentIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineOffice, RequiredTier: TierOffice,
		},
		{
			ID: "document.convert", Name: "Convert document", Description: "Move between Office and OpenDocument formats",
			Category: CategoryDocument, Icon: "convert", Verb: "Convert",
			Accepts: documentIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineOffice, RequiredTier: TierOffice,
			Options: []Option{
				{ID: "format", Label: "Convert to", Type: OptionChoice, Default: "docx", Choices: []Choice{
					{Value: "docx", Label: "Word (.docx)"},
					{Value: "odt", Label: "OpenDocument (.odt)"},
					{Value: "rtf", Label: "Rich text (.rtf)"},
					{Value: "txt", Label: "Plain text"},
				}},
				{ID: "target", Label: "Any format", Type: OptionText, Default: "", Advanced: true,
					Placeholder: "e.g. xlsx, pptx, ods, csv"},
			},
		},
	}
}

func mediaTasks() []Task {
	return []Task{
		{
			ID: "media.convert-video", Name: "Convert video", Description: "Change a video to another format",
			Category: CategoryMedia, Icon: "video", Verb: "Convert",
			Accepts: videoIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineMedia, RequiredTier: TierMedia,
			Options: []Option{
				{ID: "format", Label: "Convert to", Type: OptionChoice, Default: "mp4", Choices: []Choice{
					{Value: "mp4", Label: "MP4", Hint: "Plays on everything"},
					{Value: "webm", Label: "WEBM", Hint: "Smaller, for the web"},
					{Value: "mkv", Label: "MKV"},
					{Value: "mov", Label: "MOV"},
				}},
				{ID: "quality", Label: "Quality", Type: OptionChoice, Default: "medium", Choices: qualityChoices},
			},
		},
		{
			ID: "media.compress-video", Name: "Compress video", Description: "Make a video smaller",
			Category: CategoryMedia, Icon: "compress", Verb: "Compress",
			Accepts: videoIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineMedia, RequiredTier: TierMedia,
			Options: []Option{
				{ID: "quality", Label: "Quality", Type: OptionChoice, Default: "medium", Choices: qualityChoices},
				{ID: "maxHeight", Label: "Limit height to", Type: OptionChoice, Default: "0", Choices: []Choice{
					{Value: "0", Label: "Keep as is"},
					{Value: "1080", Label: "1080p"},
					{Value: "720", Label: "720p"},
					{Value: "480", Label: "480p"},
				}},
			},
		},
		{
			ID: "media.extract-audio", Name: "Extract audio", Description: "Take the sound out of a video",
			Category: CategoryMedia, Icon: "audio", Verb: "Extract audio",
			Accepts: videoIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineMedia, RequiredTier: TierMedia,
			Options: []Option{
				{ID: "format", Label: "Save as", Type: OptionChoice, Default: "mp3", Choices: []Choice{
					{Value: "mp3", Label: "MP3"},
					{Value: "m4a", Label: "M4A"},
					{Value: "wav", Label: "WAV", Hint: "Uncompressed"},
				}},
				{ID: "bitrate", Label: "Bitrate", Type: OptionRange, Default: 192, Min: 64, Max: 320, Step: 32, Advanced: true},
			},
		},
		{
			ID: "media.convert-audio", Name: "Convert audio", Description: "Change a sound file's format",
			Category: CategoryMedia, Icon: "audio", Verb: "Convert",
			Accepts: audioIn, MinInputs: 1, MaxInputs: 0,
			Engine: EngineMedia, RequiredTier: TierMedia,
			Options: []Option{
				{ID: "format", Label: "Convert to", Type: OptionChoice, Default: "mp3", Choices: []Choice{
					{Value: "mp3", Label: "MP3"},
					{Value: "m4a", Label: "M4A"},
					{Value: "wav", Label: "WAV"},
					{Value: "flac", Label: "FLAC", Hint: "Lossless"},
				}},
				{ID: "bitrate", Label: "Bitrate", Type: OptionRange, Default: 192, Min: 64, Max: 320, Step: 32, Advanced: true},
			},
		},
		{
			ID: "media.video-to-gif", Name: "Video to GIF", Description: "Turn a clip into an animated GIF",
			Category: CategoryMedia, Icon: "gif", Verb: "Make GIF",
			Accepts: videoIn, MinInputs: 1, MaxInputs: 1,
			Engine: EngineMedia, RequiredTier: TierMedia,
			Options: []Option{
				{ID: "start", Label: "Start at", Type: OptionText, Default: "0", Placeholder: "0:00"},
				{ID: "duration", Label: "Length", Type: OptionRange, Default: 5, Min: 1, Max: 60, Step: 1},
				{ID: "width", Label: "Width", Type: OptionChoice, Default: "480", Choices: []Choice{
					{Value: "320", Label: "Small"},
					{Value: "480", Label: "Medium"},
					{Value: "640", Label: "Large"},
				}},
			},
		},
	}
}
