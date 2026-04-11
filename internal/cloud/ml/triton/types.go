package triton

// ---------------------------------------------------------------------------
// Request / Response types used by the Client.
// These are the application-level types; the gRPC wire types are below.
// ---------------------------------------------------------------------------

// InferRequest is the application-level inference request.
type InferRequest struct {
	ModelName    string
	ModelVersion string
	TenantID     string
	RequestID    string
	Inputs       []InputTensor
	Outputs      []RequestedOutput
}

// InputTensor describes one input to the model.
type InputTensor struct {
	Name     string
	Datatype string
	Shape    []int64
	FP32Data []float32
}

// RequestedOutput names an output tensor to include in the response.
type RequestedOutput struct {
	Name string
}

// InferResponse is the application-level inference response.
type InferResponse struct {
	ModelName    string
	ModelVersion string
	ID           string
	Outputs      []OutputTensor
	LatencyMs    float64
}

// OutputTensor holds one output from the model.
type OutputTensor struct {
	Name     string
	Datatype string
	Shape    []int64
	FP32Data []float32
}

// ---------------------------------------------------------------------------
// gRPC wire types — minimal definitions matching the Triton KServe gRPC
// protocol (inference.GRPCInferenceService). These avoid a full protobuf
// dependency on the Triton proto files while remaining wire-compatible.
//
// Once buf is wired (KAI-310), replace these with generated stubs.
// ---------------------------------------------------------------------------

// ServerLiveRequest is the gRPC request for ServerLive.
type ServerLiveRequest struct{}

// ServerLiveResponse is the gRPC response for ServerLive.
type ServerLiveResponse struct {
	Live bool `protobuf:"varint,1,opt,name=live,proto3" json:"live,omitempty"`
}

// ServerReadyRequest is the gRPC request for ServerReady.
type ServerReadyRequest struct{}

// ServerReadyResponse is the gRPC response for ServerReady.
type ServerReadyResponse struct {
	Ready bool `protobuf:"varint,1,opt,name=ready,proto3" json:"ready,omitempty"`
}

// ModelReadyRequest is the gRPC request for ModelReady.
type ModelReadyRequest struct {
	Name    string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Version string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
}

// ModelReadyResponse is the gRPC response for ModelReady.
type ModelReadyResponse struct {
	Ready bool `protobuf:"varint,1,opt,name=ready,proto3" json:"ready,omitempty"`
}

// ModelInferRequest is the gRPC request for ModelInfer.
type ModelInferRequest struct {
	ModelName    string                                          `protobuf:"bytes,1,opt,name=model_name,json=modelName,proto3" json:"model_name,omitempty"`
	ModelVersion string                                          `protobuf:"bytes,2,opt,name=model_version,json=modelVersion,proto3" json:"model_version,omitempty"`
	ID           string                                          `protobuf:"bytes,3,opt,name=id,proto3" json:"id,omitempty"`
	Inputs       []*ModelInferRequest_InferInputTensor            `protobuf:"bytes,5,rep,name=inputs,proto3" json:"inputs,omitempty"`
	Outputs      []*ModelInferRequest_InferRequestedOutputTensor  `protobuf:"bytes,6,rep,name=outputs,proto3" json:"outputs,omitempty"`
}

// ModelInferRequest_InferInputTensor describes an input tensor.
type ModelInferRequest_InferInputTensor struct {
	Name     string               `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Datatype string               `protobuf:"bytes,2,opt,name=datatype,proto3" json:"datatype,omitempty"`
	Shape    []int64              `protobuf:"varint,3,rep,packed,name=shape,proto3" json:"shape,omitempty"`
	Contents *InferTensorContents `protobuf:"bytes,4,opt,name=contents,proto3" json:"contents,omitempty"`
}

// ModelInferRequest_InferRequestedOutputTensor names a requested output.
type ModelInferRequest_InferRequestedOutputTensor struct {
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

// ModelInferResponse is the gRPC response for ModelInfer.
type ModelInferResponse struct {
	ModelName    string                                   `protobuf:"bytes,1,opt,name=model_name,json=modelName,proto3" json:"model_name,omitempty"`
	ModelVersion string                                   `protobuf:"bytes,2,opt,name=model_version,json=modelVersion,proto3" json:"model_version,omitempty"`
	ID           string                                   `protobuf:"bytes,3,opt,name=id,proto3" json:"id,omitempty"`
	Outputs      []*ModelInferResponse_InferOutputTensor   `protobuf:"bytes,5,rep,name=outputs,proto3" json:"outputs,omitempty"`
}

// ModelInferResponse_InferOutputTensor describes an output tensor.
type ModelInferResponse_InferOutputTensor struct {
	Name     string               `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Datatype string               `protobuf:"bytes,2,opt,name=datatype,proto3" json:"datatype,omitempty"`
	Shape    []int64              `protobuf:"varint,3,rep,packed,name=shape,proto3" json:"shape,omitempty"`
	Contents *InferTensorContents `protobuf:"bytes,4,opt,name=contents,proto3" json:"contents,omitempty"`
}

// InferTensorContents holds the actual tensor data.
type InferTensorContents struct {
	BoolContents   []bool    `protobuf:"varint,1,rep,packed,name=bool_contents,json=boolContents,proto3" json:"bool_contents,omitempty"`
	IntContents    []int32   `protobuf:"varint,2,rep,packed,name=int_contents,json=intContents,proto3" json:"int_contents,omitempty"`
	Int64Contents  []int64   `protobuf:"varint,3,rep,packed,name=int64_contents,json=int64Contents,proto3" json:"int64_contents,omitempty"`
	Uint32Contents []uint32  `protobuf:"varint,4,rep,packed,name=uint_contents,json=uintContents,proto3" json:"uint_contents,omitempty"`
	Uint64Contents []uint64  `protobuf:"varint,5,rep,packed,name=uint64_contents,json=uint64Contents,proto3" json:"uint64_contents,omitempty"`
	Fp32Contents   []float32 `protobuf:"fixed32,6,rep,packed,name=fp32_contents,json=fp32Contents,proto3" json:"fp32_contents,omitempty"`
	Fp64Contents   []float64 `protobuf:"fixed64,7,rep,packed,name=fp64_contents,json=fp64Contents,proto3" json:"fp64_contents,omitempty"`
	BytesContents  [][]byte  `protobuf:"bytes,8,rep,name=bytes_contents,json=bytesContents,proto3" json:"bytes_contents,omitempty"`
}

// ProtoMessage / ProtoReflect / Reset stubs for gRPC wire compatibility.
// These satisfy the proto.Message interface without full protobuf codegen.

func (*ServerLiveRequest) ProtoMessage()             {}
func (*ServerLiveRequest) Reset()                    {}
func (*ServerLiveRequest) String() string            { return "ServerLiveRequest" }
func (*ServerLiveResponse) ProtoMessage()            {}
func (*ServerLiveResponse) Reset()                   {}
func (*ServerLiveResponse) String() string           { return "ServerLiveResponse" }
func (*ServerReadyRequest) ProtoMessage()            {}
func (*ServerReadyRequest) Reset()                   {}
func (*ServerReadyRequest) String() string           { return "ServerReadyRequest" }
func (*ServerReadyResponse) ProtoMessage()           {}
func (*ServerReadyResponse) Reset()                  {}
func (*ServerReadyResponse) String() string          { return "ServerReadyResponse" }
func (*ModelReadyRequest) ProtoMessage()             {}
func (*ModelReadyRequest) Reset()                    {}
func (*ModelReadyRequest) String() string            { return "ModelReadyRequest" }
func (*ModelReadyResponse) ProtoMessage()            {}
func (*ModelReadyResponse) Reset()                   {}
func (*ModelReadyResponse) String() string           { return "ModelReadyResponse" }
func (*ModelInferRequest) ProtoMessage()             {}
func (*ModelInferRequest) Reset()                    {}
func (*ModelInferRequest) String() string            { return "ModelInferRequest" }
func (*ModelInferResponse) ProtoMessage()            {}
func (*ModelInferResponse) Reset()                   {}
func (*ModelInferResponse) String() string           { return "ModelInferResponse" }
func (*InferTensorContents) ProtoMessage()           {}
func (*InferTensorContents) Reset()                  {}
func (*InferTensorContents) String() string          { return "InferTensorContents" }
func (*ModelInferRequest_InferInputTensor) ProtoMessage()  {}
func (*ModelInferRequest_InferInputTensor) Reset()         {}
func (*ModelInferRequest_InferInputTensor) String() string { return "InferInputTensor" }
func (*ModelInferRequest_InferRequestedOutputTensor) ProtoMessage()  {}
func (*ModelInferRequest_InferRequestedOutputTensor) Reset()         {}
func (*ModelInferRequest_InferRequestedOutputTensor) String() string { return "InferRequestedOutputTensor" }
func (*ModelInferResponse_InferOutputTensor) ProtoMessage()  {}
func (*ModelInferResponse_InferOutputTensor) Reset()         {}
func (*ModelInferResponse_InferOutputTensor) String() string { return "InferOutputTensor" }
