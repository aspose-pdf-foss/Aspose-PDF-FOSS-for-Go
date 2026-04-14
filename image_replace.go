package asposepdf

import (
	"fmt"
	"io"
	"os"
)

// Replace replaces the image data with a new image from a file.
// Format is detected by magic bytes (JPEG, PNG). Dimensions may change.
// Position and size on the page remain unchanged.
func (info *ImageInfo) Replace(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("replace image: %w", err)
	}
	return info.replaceFromBytes(data)
}

// ReplaceFromStream replaces the image data with a new image from a reader.
func (info *ImageInfo) ReplaceFromStream(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("replace image: %w", err)
	}
	return info.replaceFromBytes(data)
}

func (info *ImageInfo) replaceFromBytes(data []byte) error {
	if info.stream == nil {
		return fmt.Errorf("image info: no image data")
	}
	if len(data) == 0 {
		return fmt.Errorf("replace image: empty data")
	}

	format, err := detectImageFormat(data)
	if err != nil {
		return err
	}

	newStream, newSmask, err := createImageXObject(data, format)
	if err != nil {
		return err
	}

	// Update existing stream in place.
	info.stream.Data = newStream.Data
	info.stream.Decoded = newStream.Decoded

	// Replace dict fields from new stream.
	info.stream.Dict["/Width"] = newStream.Dict["/Width"]
	info.stream.Dict["/Height"] = newStream.Dict["/Height"]
	info.stream.Dict["/BitsPerComponent"] = newStream.Dict["/BitsPerComponent"]
	info.stream.Dict["/ColorSpace"] = newStream.Dict["/ColorSpace"]

	// Handle /Filter transition.
	if f, ok := newStream.Dict["/Filter"]; ok {
		info.stream.Dict["/Filter"] = f
	} else {
		delete(info.stream.Dict, "/Filter")
	}
	delete(info.stream.Dict, "/DecodeParms")

	// Handle SMask transition.
	if newSmask != nil {
		// Register new SMask in document objects.
		smaskID := info.page.doc.nextID
		info.page.doc.nextID++
		info.page.doc.objects[smaskID] = &pdfObject{Num: smaskID, Value: newSmask}
		info.stream.Dict["/SMask"] = pdfRef{Num: smaskID}
	} else {
		delete(info.stream.Dict, "/SMask")
	}

	return nil
}
