package capability

// Category is the top-level HuggingFace domain for ML tasks.
type Category string

const (
	CategoryMultimodal     Category = "multimodal"
	CategoryComputerVision Category = "computer-vision"
	CategoryNLP            Category = "nlp"
	CategoryAudio          Category = "audio"
	CategoryTabular        Category = "tabular"
	CategoryRL             Category = "reinforcement-learning"
)

// Task is a specific ML task identifier, aligned with HuggingFace pipeline
// identifiers where they exist so external documentation maps 1:1.
type Task string

const (
	// ── Multimodal ───────────────────────────────────────────────────────
	ImageTextToText           Task = "image-text-to-text"
	VisualQuestionAnswering   Task = "visual-question-answering"
	DocumentQuestionAnswering Task = "document-question-answering"
	AnyToAny                  Task = "any-to-any"

	// ── Computer Vision — understanding ──────────────────────────────────
	ImageClassification         Task = "image-classification"
	ObjectDetection             Task = "object-detection"
	ImageSegmentation           Task = "image-segmentation"
	DepthEstimation             Task = "depth-estimation"
	VideoClassification         Task = "video-classification"
	ZeroShotImageClassification Task = "zero-shot-image-classification"
	ZeroShotObjectDetection     Task = "zero-shot-object-detection"
	KeypointMatching            Task = "keypoint-matching"
	MaskGeneration              Task = "mask-generation"

	// ── Computer Vision — generation ─────────────────────────────────────
	TextToImage                  Task = "text-to-image"
	ImageToImage                 Task = "image-to-image"
	UnconditionalImageGeneration Task = "unconditional-image-generation"

	// ── NLP ───────────────────────────────────────────────────────────────
	TextGeneration      Task = "text-generation"
	TextClassification  Task = "text-classification"
	TokenClassification Task = "token-classification"
	FillMask            Task = "fill-mask"
	FeatureExtraction   Task = "feature-extraction"

	// ── Audio — understanding ─────────────────────────────────────────────
	AutomaticSpeechRecognition  Task = "automatic-speech-recognition"
	AudioClassification         Task = "audio-classification"
	ZeroShotAudioClassification Task = "zero-shot-audio-classification"

	// ── Audio — generation ────────────────────────────────────────────────
	TextToAudio Task = "text-to-audio" // alias: text-to-speech
)

// Contract answers "which delivery interface does this task use?"
// Maps to the agent interface tree from Draft 38.
type Contract string

const (
	ContractChange      Contract = "change"      // CodingAgent.Deliver
	ContractArtifact    Contract = "artifact"    // AuthoringAgent.Artifacts
	ContractObservation Contract = "observation" // ObservingAgent.Observations
)

// Modality is an input or output data type that a model must support.
// Aligns with Draft 41's model capability flags.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityVideo Modality = "video"
	ModalityAudio Modality = "audio"
)
