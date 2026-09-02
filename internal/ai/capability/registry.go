package capability

// Spec carries static metadata about a task.
type Spec struct {
	Category         Category
	InputModalities  []Modality
	OutputModalities []Modality
	DeliveryContract Contract
}

// Registry is the single source of truth for task metadata.
// An AST test asserts every Task constant appears here before CI goes green.
var Registry = map[Task]Spec{
	// ── Multimodal ───────────────────────────────────────────────────────
	ImageTextToText: {
		Category:         CategoryMultimodal,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	VisualQuestionAnswering: {
		Category:         CategoryMultimodal,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	DocumentQuestionAnswering: {
		Category:         CategoryMultimodal,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	AnyToAny: {
		Category:         CategoryMultimodal,
		InputModalities:  []Modality{ModalityText, ModalityImage, ModalityAudio, ModalityVideo},
		OutputModalities: []Modality{ModalityText, ModalityImage, ModalityAudio, ModalityVideo},
		DeliveryContract: ContractArtifact,
	},

	// ── Computer Vision — understanding ──────────────────────────────────
	ImageClassification: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	ObjectDetection: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	ImageSegmentation: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	DepthEstimation: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	VideoClassification: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityVideo},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	ZeroShotImageClassification: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	ZeroShotObjectDetection: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	KeypointMatching: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	MaskGeneration: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},

	// ── Computer Vision — generation ─────────────────────────────────────
	TextToImage: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityImage},
		DeliveryContract: ContractArtifact,
	},
	ImageToImage: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{ModalityImage, ModalityText},
		OutputModalities: []Modality{ModalityImage},
		DeliveryContract: ContractArtifact,
	},
	UnconditionalImageGeneration: {
		Category:         CategoryComputerVision,
		InputModalities:  []Modality{},
		OutputModalities: []Modality{ModalityImage},
		DeliveryContract: ContractArtifact,
	},

	// ── NLP ───────────────────────────────────────────────────────────────
	TextGeneration: {
		Category:         CategoryNLP,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractArtifact, // or ContractChange when used by coding CLI
	},
	TextClassification: {
		Category:         CategoryNLP,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	TokenClassification: {
		Category:         CategoryNLP,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	FillMask: {
		Category:         CategoryNLP,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	FeatureExtraction: {
		Category:         CategoryNLP,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},

	// ── Audio — understanding ─────────────────────────────────────────────
	AutomaticSpeechRecognition: {
		Category:         CategoryAudio,
		InputModalities:  []Modality{ModalityAudio},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	AudioClassification: {
		Category:         CategoryAudio,
		InputModalities:  []Modality{ModalityAudio},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},
	ZeroShotAudioClassification: {
		Category:         CategoryAudio,
		InputModalities:  []Modality{ModalityAudio, ModalityText},
		OutputModalities: []Modality{ModalityText},
		DeliveryContract: ContractObservation,
	},

	// ── Audio — generation ────────────────────────────────────────────────
	TextToAudio: {
		Category:         CategoryAudio,
		InputModalities:  []Modality{ModalityText},
		OutputModalities: []Modality{ModalityAudio},
		DeliveryContract: ContractArtifact,
	},
}

// Tasks returns all registered task identifiers.
func Tasks() []Task {
	tasks := make([]Task, 0, len(Registry))
	for task := range Registry {
		tasks = append(tasks, task)
	}
	return tasks
}

// Categories returns all category identifiers.
func Categories() []Category {
	return []Category{
		CategoryMultimodal,
		CategoryComputerVision,
		CategoryNLP,
		CategoryAudio,
		CategoryTabular,
		CategoryRL,
	}
}

// IsValid reports whether a task is registered.
func IsValid(t Task) bool {
	_, ok := Registry[t]
	return ok
}
