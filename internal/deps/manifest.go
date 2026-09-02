package deps

// The component manifest is compiled into the binary. Every checksum here was
// taken from the publisher or computed from the exact file at the pinned URL —
// none is a placeholder, and the installer refuses any component whose
// checksum is empty rather than trusting an unverified download.
//
// Not every dependency can be distributed this way. FFmpeg publishes portable,
// checksummed static builds for all three platforms, so Lathe downloads it.
// Tesseract and LibreOffice ship as platform installers rather than portable
// archives, so Lathe detects an existing installation instead of downloading
// one, and says plainly what to install when it finds nothing. See
// docs/BUNDLING.md.

// ffmpegVersion is the release pinned for Windows and macOS. Linux static
// builds come from a different publisher on its own schedule, so its version
// is recorded on that Source instead. Changing any of this means recomputing
// the checksums; scripts/checksums does that.
const ffmpegVersion = "9.0.1"

// Manifest returns every component Lathe knows about.
func Manifest() []Component {
	return []Component{ffmpeg(), tesseract(), libreOffice(), tessdata()}
}

func ffmpeg() Component {
	return Component{
		ID:          "ffmpeg",
		Tier:        TierMedia,
		DisplayName: "Video and photo support",
		Explanation: "Converting video, extracting audio and reading HEIC photos from a phone " +
			"use FFmpeg, a free open-source media toolkit. It downloads once and works offline afterwards.",
		DownloadBytes:  111253802,
		InstalledBytes: 300 << 20,
		Version:        ffmpegVersion,
		Binaries:       []string{"ffmpeg", "ffprobe"},
		VersionArgs:    []string{"-version"},

		Sources: map[string]Source{
			"windows/amd64": {
				URL:         "https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-" + ffmpegVersion + "-essentials_build.zip",
				SHA256:      "fec81ae03971d9dd4be3ebe02e263bd2ec1d789483f931bdba5f5715e65da2e9",
				StripPrefix: "*/",
				Version:     ffmpegVersion,
			},
			// evermeet publishes Intel builds; Apple silicon runs them under
			// Rosetta, which is transparent and fast enough for conversion.
			"darwin/amd64": {
				URL:     "https://evermeet.cx/ffmpeg/ffmpeg-" + ffmpegVersion + ".zip",
				SHA256:  "8a8c9e549983409fe6604b9aa665648b7a5def9407fe814c39c8b2ea7f64a48f",
				Version: ffmpegVersion,
			},
			"darwin/arm64": {
				URL:     "https://evermeet.cx/ffmpeg/ffmpeg-" + ffmpegVersion + ".zip",
				SHA256:  "8a8c9e549983409fe6604b9aa665648b7a5def9407fe814c39c8b2ea7f64a48f",
				Version: ffmpegVersion,
			},
			// John Van Sickle publishes only a rolling "release" URL with no
			// versioned archive, so this checksum has to be refreshed whenever
			// upstream publishes. A mismatch refuses the install rather than
			// trusting it, which is the correct failure.
			"linux/amd64": {
				URL:         "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz",
				SHA256:      "abda8d77ce8309141f83ab8edf0596834087c52467f6badf376a6a2a4c87cf67",
				StripPrefix: "*/",
				Version:     "7.0.2",
				Rolling:     true,
			},
		},
	}
}

// tesseract is detected rather than downloaded: no publisher offers a
// portable, checksummed archive for all three platforms, and shipping an
// unverified binary would undo the point of the checksum rule.
func tesseract() Component {
	return Component{
		ID:          "tesseract",
		Tier:        TierBundled,
		DisplayName: "Text recognition",
		Explanation: "Reading text out of images uses Tesseract, a free open-source OCR engine.",
		Version:     "system",
		Binaries:    []string{"tesseract"},
		VersionArgs: []string{"--version"},
		SystemOnly:  true,
		SearchPaths: []string{
			`C:\Program Files\Tesseract-OCR`,
			`C:\Program Files (x86)\Tesseract-OCR`,
			"/usr/bin", "/usr/local/bin",
			"/opt/homebrew/bin", "/opt/local/bin",
			"/snap/bin",
		},
		InstallHint: map[string]string{
			"windows": "Install Tesseract from github.com/UB-Mannheim/tesseract, then restart Lathe.",
			"darwin":  "Install Tesseract with: brew install tesseract",
			"linux":   "Install Tesseract with your package manager, for example: sudo apt install tesseract-ocr",
		},
	}
}

// libreOffice is detected rather than downloaded, for the same reason as
// Tesseract: the official builds are platform installers, not archives.
func libreOffice() Component {
	return Component{
		ID:          "libreoffice",
		Tier:        TierOffice,
		DisplayName: "Office document support",
		Explanation: "Converting Word, Excel and PowerPoint files uses LibreOffice, a free open-source office suite.",
		Version:     "system",
		Binaries:    []string{"soffice"},
		VersionArgs: []string{"--version"},
		WindowsExt:  ".com",
		SystemOnly:  true,
		SearchPaths: []string{
			`C:\Program Files\LibreOffice\program`,
			`C:\Program Files (x86)\LibreOffice\program`,
			"/Applications/LibreOffice.app/Contents/MacOS",
			"/usr/bin", "/usr/local/bin",
			"/opt/libreoffice/program",
			"/snap/bin",
		},
		InstallHint: map[string]string{
			"windows": "Install LibreOffice from libreoffice.org, then restart Lathe.",
			"darwin":  "Install LibreOffice from libreoffice.org, or with: brew install --cask libreoffice",
			"linux":   "Install LibreOffice with your package manager, for example: sudo apt install libreoffice",
		},
	}
}

// tessdata carries the English language model. Additional languages are
// separate components so someone who only reads English never downloads them;
// see Languages.
func tessdata() Component {
	return Component{
		ID:             "tessdata-eng",
		Tier:           TierBundled,
		DisplayName:    "English text recognition",
		Explanation:    "The language model Tesseract uses to read English text.",
		DownloadBytes:  4113088,
		InstalledBytes: 4113088,
		Version:        "tessdata_fast",
		Sources: map[string]Source{
			"any": {
				URL:    "https://github.com/tesseract-ocr/tessdata_fast/raw/main/eng.traineddata",
				SHA256: "7d4322bd2a7749724879683fc3912cb542f19906c83bcc1a52132556427170b2",
			},
		},
	}
}

// Language is an additional OCR language pack.
type Language struct {
	Code string
	Name string
}

// Languages lists the OCR languages Lathe offers beyond English. Indian
// documents routinely mix scripts, and Tesseract can read several in one pass,
// so these are the ones worth offering first.
func Languages() []Language {
	return []Language{
		{"hin", "Hindi"},
		{"ben", "Bengali"},
		{"tam", "Tamil"},
		{"tel", "Telugu"},
		{"mar", "Marathi"},
		{"guj", "Gujarati"},
		{"kan", "Kannada"},
		{"mal", "Malayalam"},
		{"pan", "Punjabi"},
		{"urd", "Urdu"},
		{"ara", "Arabic"},
		{"deu", "German"},
		{"fra", "French"},
		{"spa", "Spanish"},
		{"por", "Portuguese"},
		{"rus", "Russian"},
		{"jpn", "Japanese"},
		{"chi_sim", "Chinese (Simplified)"},
	}
}
