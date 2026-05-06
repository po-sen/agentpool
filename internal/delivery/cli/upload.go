package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxUploadFiles          = 500
	maxUploadFileSizeBytes  = 50 << 20
	maxTotalUploadSizeBytes = 200 << 20

	formatUnsafeFilePath     = "unsafe file path: %s"
	formatUploadFileTooLarge = "upload file exceeds %d bytes: %s"
)

var textUploadExtensions = map[string]struct{}{
	".csv":  {},
	".css":  {},
	".go":   {},
	".html": {},
	".js":   {},
	".json": {},
	".jsx":  {},
	".md":   {},
	".mod":  {},
	".py":   {},
	".sh":   {},
	".sum":  {},
	".toml": {},
	".ts":   {},
	".tsx":  {},
	".txt":  {},
	".xml":  {},
	".yaml": {},
	".yml":  {},
}

var binaryUploadExtensions = map[string]struct{}{
	".bmp":  {},
	".doc":  {},
	".docx": {},
	".gif":  {},
	".heic": {},
	".jpeg": {},
	".jpg":  {},
	".odp":  {},
	".ods":  {},
	".odt":  {},
	".pdf":  {},
	".png":  {},
	".ppt":  {},
	".pptx": {},
	".rtf":  {},
	".tif":  {},
	".tiff": {},
	".webp": {},
	".xls":  {},
	".xlsx": {},
}

var textUploadBasenames = map[string]struct{}{
	".dockerignore": {},
	".env.example":  {},
	".gitignore":    {},
	"dockerfile":    {},
	"makefile":      {},
}

var excludedUploadDirs = map[string]struct{}{
	".cache":       {},
	".git":         {},
	".hg":          {},
	".next":        {},
	".svn":         {},
	".turbo":       {},
	"build":        {},
	"dist":         {},
	"node_modules": {},
	"target":       {},
	"vendor":       {},
}

type uploadFile struct {
	filename string
	content  []byte
}

type uploadCollector struct {
	files     []uploadFile
	seen      map[string]struct{}
	totalSize int64
}

func collectUploadInputs(request CreateRunRequest) ([]uploadFile, error) {
	collector := uploadCollector{seen: make(map[string]struct{})}
	for _, filePath := range request.Files {
		if err := collector.addFilePath(filePath); err != nil {
			return nil, err
		}
	}
	for _, dirPath := range request.Dirs {
		if err := collector.addDirectory(dirPath); err != nil {
			return nil, err
		}
	}
	for _, archivePath := range request.Archives {
		if err := collector.addArchive(archivePath); err != nil {
			return nil, err
		}
	}

	return collector.files, nil
}

func (c *uploadCollector) addFilePath(filePath string) error {
	filename, err := uploadFilename(filePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(filePath) //nolint:gosec // CLI intentionally reads user-selected upload paths.
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("upload path is not a regular file: %s", filePath)
	}
	content, err := readUploadFile(filePath, info.Size())
	if err != nil {
		return err
	}

	return c.addUpload(filename, content)
}

func (c *uploadCollector) addDirectory(dirPath string) error {
	root, skip, err := uploadDirectoryRoot(dirPath)
	if err != nil || skip {
		return err
	}

	return filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == root {
			return nil
		}

		return c.addDirectoryEntry(root, current, entry)
	})
}

func uploadDirectoryRoot(dirPath string) (string, bool, error) {
	if strings.TrimSpace(dirPath) == "" {
		return "", false, errors.New("directory path is required")
	}
	root, err := filepath.Abs(dirPath)
	if err != nil {
		return "", false, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", false, err
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("upload directory is not a directory: %s", dirPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("upload directory is a symlink: %s", dirPath)
	}
	if excludedUploadDir(filepath.Base(root)) {
		return root, true, nil
	}

	return root, false, nil
}

func (c *uploadCollector) addDirectoryEntry(root string, current string, entry os.DirEntry) error {
	if entry.IsDir() {
		if excludedUploadDir(entry.Name()) {
			return filepath.SkipDir
		}

		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	filename, err := directoryUploadFilename(root, current)
	if err != nil || uploadPathContainsExcludedDir(filename) {
		return err
	}
	if !supportedUploadName(filename) {
		return nil
	}
	content, err := readUploadFile(current, info.Size())
	if err != nil {
		return err
	}

	return c.addUpload(filename, content)
}

func directoryUploadFilename(root string, current string) (string, error) {
	relative, err := filepath.Rel(root, current)
	if err != nil {
		return "", err
	}

	return filepath.ToSlash(relative), nil
}

func (c *uploadCollector) addArchive(archivePath string) error {
	if strings.TrimSpace(archivePath) == "" {
		return errors.New("archive path is required")
	}
	normalized := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(normalized, ".tar.gz") || strings.HasSuffix(normalized, ".tgz"):
		return c.addTarArchive(archivePath, true)
	case strings.HasSuffix(normalized, ".tar"):
		return c.addTarArchive(archivePath, false)
	case strings.HasSuffix(normalized, ".zip"):
		return c.addZipArchive(archivePath)
	default:
		return fmt.Errorf("unsupported archive type: %s", archivePath)
	}
}

func (c *uploadCollector) addTarArchive(archivePath string, gzipCompressed bool) error {
	source, err := os.Open(archivePath) //nolint:gosec // CLI intentionally reads user-selected upload paths.
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	var reader io.Reader = source
	if gzipCompressed {
		gzipReader, err := gzip.NewReader(source)
		if err != nil {
			return err
		}
		defer func() {
			_ = gzipReader.Close()
		}()
		reader = gzipReader
	}

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := c.addTarEntry(tarReader, header); err != nil {
			return err
		}
	}

	return nil
}

func (c *uploadCollector) addTarEntry(reader io.Reader, header *tar.Header) error {
	if header.Typeflag != tar.TypeReg {
		return nil
	}

	filename, err := archiveUploadFilename(header.Name)
	if err != nil {
		return err
	}
	if uploadPathContainsExcludedDir(filename) {
		return nil
	}
	if !supportedUploadName(filename) {
		return nil
	}
	if header.Size > maxUploadFileSizeBytes {
		return fmt.Errorf(formatUploadFileTooLarge, maxUploadFileSizeBytes, filename)
	}
	content, err := readLimitedUploadContent(reader, header.Size, filename)
	if err != nil {
		return err
	}

	return c.addUpload(filename, content)
}

func (c *uploadCollector) addZipArchive(archivePath string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()

	for _, file := range reader.File {
		if err := c.addZipFile(file); err != nil {
			return err
		}
	}

	return nil
}

func (c *uploadCollector) addZipFile(file *zip.File) error {
	info := file.FileInfo()
	if !info.Mode().IsRegular() {
		return nil
	}
	filename, err := archiveUploadFilename(file.Name)
	if err != nil {
		return err
	}
	if uploadPathContainsExcludedDir(filename) {
		return nil
	}
	if !supportedUploadName(filename) {
		return nil
	}
	if info.Size() > maxUploadFileSizeBytes {
		return fmt.Errorf(formatUploadFileTooLarge, maxUploadFileSizeBytes, filename)
	}
	source, err := file.Open()
	if err != nil {
		return err
	}
	content, readErr := readLimitedUploadContent(source, info.Size(), filename)
	closeErr := source.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}

	return c.addUpload(filename, content)
}

func (c *uploadCollector) addUpload(filename string, content []byte) error {
	if !safeRelativeFilename(filename) {
		return fmt.Errorf(formatUnsafeFilePath, filename)
	}
	if !supportedUploadName(filename) {
		return fmt.Errorf("unsupported upload file type: %s", filename)
	}
	if len(content) > maxUploadFileSizeBytes {
		return fmt.Errorf(formatUploadFileTooLarge, maxUploadFileSizeBytes, filename)
	}
	if uploadRequiresUTF8(filename) && !utf8.Valid(content) {
		return fmt.Errorf("upload file must be UTF-8 text: %s", filename)
	}
	if _, ok := c.seen[filename]; ok {
		return fmt.Errorf("duplicate upload path: %s", filename)
	}
	if len(c.files)+1 > maxUploadFiles {
		return fmt.Errorf("too many upload files: maximum is %d", maxUploadFiles)
	}
	nextTotal := c.totalSize + int64(len(content))
	if nextTotal > maxTotalUploadSizeBytes {
		return fmt.Errorf("uploads exceed %d bytes total", maxTotalUploadSizeBytes)
	}

	c.seen[filename] = struct{}{}
	c.files = append(c.files, uploadFile{filename: filename, content: append([]byte(nil), content...)})
	c.totalSize = nextTotal

	return nil
}

func readUploadFile(filePath string, size int64) ([]byte, error) {
	if size > maxUploadFileSizeBytes {
		return nil, fmt.Errorf(formatUploadFileTooLarge, maxUploadFileSizeBytes, filePath)
	}
	source, err := os.Open(filePath) //nolint:gosec // CLI intentionally reads user-selected upload paths.
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = source.Close()
	}()

	return readLimitedUploadContent(source, size, filePath)
}

func readLimitedUploadContent(reader io.Reader, size int64, name string) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("upload file has invalid size: %s", name)
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxUploadFileSizeBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxUploadFileSizeBytes {
		return nil, fmt.Errorf(formatUploadFileTooLarge, maxUploadFileSizeBytes, name)
	}
	if size > 0 && int64(len(content)) != size {
		return nil, fmt.Errorf("upload file size changed while reading: %s", name)
	}

	return content, nil
}

func archiveUploadFilename(name string) (string, error) {
	filename := filepath.ToSlash(name)
	if filename == "" || strings.TrimSpace(filename) == "" {
		return "", errors.New("archive entry filename is required")
	}
	if filename != strings.TrimSpace(filename) ||
		path.IsAbs(filename) ||
		strings.Contains(filename, "\\") ||
		strings.Contains(filename, ":") ||
		filepath.VolumeName(filename) != "" ||
		archivePathContainsParentTraversal(filename) {
		return "", fmt.Errorf("unsafe archive entry path: %s", name)
	}

	filename = path.Clean(filename)
	if filename == "." || !safeRelativeFilename(filename) {
		return "", fmt.Errorf("unsafe archive entry path: %s", name)
	}

	return filename, nil
}

func archivePathContainsParentTraversal(filename string) bool {
	for _, component := range strings.Split(filename, "/") {
		if component == ".." {
			return true
		}
	}

	return false
}

func uploadFilename(filePath string) (string, error) {
	if strings.TrimSpace(filePath) == "" {
		return "", errors.New("file path is required")
	}
	if filepath.IsAbs(filePath) {
		return safeBaseFilename(filePath)
	}

	name := filepath.ToSlash(filePath)
	if !safeRelativeFilename(name) {
		return "", fmt.Errorf(formatUnsafeFilePath, filePath)
	}

	return name, nil
}

func safeBaseFilename(filePath string) (string, error) {
	name := filepath.Base(filePath)
	if !safeRelativeFilename(name) {
		return "", fmt.Errorf(formatUnsafeFilePath, filePath)
	}

	return name, nil
}

func safeRelativeFilename(name string) bool {
	if name == "" ||
		name != strings.TrimSpace(name) ||
		strings.HasPrefix(name, "/") ||
		strings.Contains(name, "\\") ||
		strings.Contains(name, ":") ||
		filepath.VolumeName(name) != "" {
		return false
	}
	if path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}

	return true
}

func supportedUploadName(filename string) bool {
	base := strings.ToLower(path.Base(filename))
	if _, ok := textUploadBasenames[base]; ok {
		return true
	}
	extension := strings.ToLower(path.Ext(filename))
	if _, ok := textUploadExtensions[extension]; ok {
		return true
	}
	_, ok := binaryUploadExtensions[extension]

	return ok
}

func uploadRequiresUTF8(filename string) bool {
	base := strings.ToLower(path.Base(filename))
	if _, ok := textUploadBasenames[base]; ok {
		return true
	}
	_, ok := textUploadExtensions[strings.ToLower(path.Ext(filename))]

	return ok
}

func excludedUploadDir(name string) bool {
	_, ok := excludedUploadDirs[strings.ToLower(name)]

	return ok
}

func uploadPathContainsExcludedDir(filename string) bool {
	for _, component := range strings.Split(filename, "/") {
		if excludedUploadDir(component) {
			return true
		}
	}

	return false
}
