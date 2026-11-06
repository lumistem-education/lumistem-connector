package model

type CreateJobRequest struct {
	BrowserUpload bool     `json:"browserUpload,omitempty"`
	Code          string   `json:"code"`
	Board         string   `json:"board"`
	Libs          []string `json:"libs,omitempty"`
}

type CreateJobResponse struct {
	JobID string `json:"jobId"`
	Token string `json:"token"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

type PortInfo struct {
	Address  string `json:"address"`
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
	Board    string `json:"board,omitempty"`
}

type LibraryInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Author  string `json:"author,omitempty"`
}

type InstallLibraryRequest struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type Notice struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Core    string `json:"core,omitempty"`
}

type OpenSerialRequest struct {
	Port     string `json:"port"`
	BaudRate int    `json:"baudRate"`
}

type OpenSerialResponse struct {
	SessionID string `json:"sessionId"`
}

type WriteSerialRequest struct {
	Data string `json:"data"`
}
