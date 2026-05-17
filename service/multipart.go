package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
)

// MultipartWriter wraps multipart.Writer to support custom content types on form files.
type MultipartWriter struct {
	*multipart.Writer
}

// NewMultipartWriter creates a new MultipartWriter.
func NewMultipartWriter(w io.Writer) *MultipartWriter {
	return &MultipartWriter{multipart.NewWriter(w)}
}

// CreateFormFile creates a form file part with a custom content type.
func (mw *MultipartWriter) CreateFormFile(fieldname, filename, contentType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldname, filename))
	h.Set("Content-Type", contentType)
	return mw.Writer.CreatePart(h)
}
