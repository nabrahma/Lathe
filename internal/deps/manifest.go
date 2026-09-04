package deps

// The component manifest is compiled into the binary. Every checksum here was
// taken from the publisher or computed from the exact file at the pinned URL.
// None is a placeholder, and the installer refuses any component whose
// checksum is empty rather than trusting an unverified download.
//
// Not every dependency arrives the same way, and which way is a property of
// the platform rather than of the project. FFmpeg publishes portable static
// builds everywhere, so Lathe unpacks an archive. Tesseract and Ghostscript
// publish a Windows setup program and nothing portable, so there Lathe runs
// the installer, pinned by checksum like everything else, into its own folder.
// On macOS and Linux those two are a package manager away, which needs a root
// password Lathe has no business asking for, so they are detected instead and
// Lathe says exactly what to run. LibreOffice is detected everywhere. See
// docs/BUNDLING.md.

// ffmpegVersion is the release pinned for Windows and macOS. Linux static
// builds come from a different publisher on its own schedule, so its version
// is recorded on that Source instead. Changing any of this means recomputing
// the checksums, which is `curl -sL <url> | sha256sum` for each one changed.
const ffmpegVersion = "9.0.1"

// Manifest returns every component Lathe knows about.
func Manifest() []Component {
	return []Component{ffmpeg(), tesseract(), libreOffice(), tessdata(), ghostscript()}
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
		// An FFmpeg already on the machine is used as-is; the download is only
		// for people who do not have one.
		SearchPaths: []string{
			"/usr/bin", "/usr/local/bin", "/opt/homebrew/bin", "/snap/bin",
			`C:\Program Files\ffmpeg\bin`,
		},

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

// tesseractVersion is the UB-Mannheim build pinned for Windows. It is taken
// from their GitHub release rather than their own download host, which refuses
// requests that do not come from a browser: a download button pointing at a
// server that resets the connection is worse than no button.
const tesseractVersion = "5.4.0.20240606"

// tesseract downloads on Windows and is detected everywhere else. UB-Mannheim
// publishes a setup program, which Lathe runs into its own component folder;
// on macOS and Linux the answer is a package manager, which needs a root
// password Lathe should not be asking for.
func tesseract() Component {
	return Component{
		ID:             "tesseract",
		Tier:           TierBundled,
		DisplayName:    "Text recognition",
		Explanation:    "Reading text out of images uses Tesseract, a free open-source OCR engine.",
		Version:        "system",
		Binaries:       []string{"tesseract"},
		VersionArgs:    []string{"--version"},
		DownloadBytes:  50175248,
		InstalledBytes: 180 << 20,
		Sources: map[string]Source{
			"windows/amd64": {
				URL: "https://github.com/UB-Mannheim/tesseract/releases/download/v" +
					tesseractVersion + "/tesseract-ocr-w64-setup-" + tesseractVersion + ".exe",
				SHA256:  "c885fff6998e0608ba4bb8ab51436e1c6775c2bafc2559a19b423e18678b60c9",
				Version: tesseractVersion,
				// NSIS: /S is silent and /D is the destination. /D takes the
				// rest of the line literally, so it goes last and unquoted.
				InstallerArgs: []string{"/S", "/D=" + dirPlaceholder},
				// The manifest asks for the highest rights the account holds,
				// so an administrator sees a permission prompt.
				Elevates: true,
			},
		},
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

// ghostscriptVersion is the Artifex build pinned for Windows.
const ghostscriptVersion = "10.07.1"

// ghostscript makes Compress PDF markedly better when it is present, and is
// deliberately optional: no task requires it, so a PDF still compresses
// without it using the built-in path. Artifex publishes a Windows installer
// and, elsewhere, source only, so it downloads on Windows and is detected on
// the platforms where the answer is a package manager.
func ghostscript() Component {
	return Component{
		ID:          "ghostscript",
		Tier:        TierEnhance,
		DisplayName: "Stronger PDF compression",
		Explanation: "Ghostscript, a free open-source PDF engine, can lower the resolution of " +
			"scanned pages as well as their quality. Without it Lathe still compresses PDFs, " +
			"just less far.",
		Version:        "system",
		Binaries:       []string{"gs"},
		VersionArgs:    []string{"--version"},
		DownloadBytes:  64966216,
		InstalledBytes: 130 << 20,
		Sources: map[string]Source{
			"windows/amd64": {
				URL:           "https://github.com/ArtifexSoftware/ghostpdl-downloads/releases/download/gs10071/gs10071w64.exe",
				SHA256:        "3a4c28d0aac47aa7cccd35a5932c55110376e9dbd966898dde388b7faba444a4",
				Version:       ghostscriptVersion,
				InstallerArgs: []string{"/S", "/D=" + dirPlaceholder},
				// This one asks for administrator outright, so the prompt
				// appears for every account.
				Elevates: true,
			},
		},
		// The console build is named differently on Windows, and gswin64c is
		// the one that writes to stdout instead of opening a window.
		WindowsNames: map[string]string{"gs": "gswin64c"},
		SearchPaths: []string{
			`C:\Program Files\gs`,
			`C:\Program Files (x86)\gs`,
			"/usr/bin", "/usr/local/bin",
			"/opt/homebrew/bin", "/opt/local/bin",
		},
		InstallHint: map[string]string{
			"windows": "Install Ghostscript from ghostscript.com/releases, then restart Lathe.",
			"darwin":  "Install Ghostscript with: brew install ghostscript",
			"linux":   "Install Ghostscript with your package manager, for example: sudo apt install ghostscript",
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
