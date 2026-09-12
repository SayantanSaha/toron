package ablation

// ConditionADocs holds the cross-referenced documentation identifiers for Condition A.
type ConditionADocs struct {
	Req  string `json:"req"`
	Task string `json:"task"`
	Adr  string `json:"adr"`
	Tc   string `json:"tc"`
	Cr   string `json:"cr"`
	Sr   string `json:"sr"`
}

// TaskDef defines an evaluated systems engineering task in the 10-task cohort.
type TaskDef struct {
	ID                  string         `json:"id"`
	Title               string         `json:"title"`
	Subsystem           string         `json:"subsystem"`
	TargetVulnerability string         `json:"target_vulnerability"`
	AcceptanceCriteria  []string       `json:"acceptance_criteria"`
	ConditionADocs      ConditionADocs `json:"condition_a_docs"`
}

// ConditionMetrics captures the quantitative evaluation results for a single task under one condition.
type ConditionMetrics struct {
	Condition                string   `json:"condition"`
	TotalCriteria            int      `json:"total_criteria"`
	ViolatedCriteria         int      `json:"violated_criteria"`
	DriftRatePercent         float64  `json:"drift_rate_percent"`
	ProtocolOmissions        int      `json:"protocol_omissions"`
	SecurityVulnerabilities   int      `json:"security_vulnerabilities"`
	Regressions              int      `json:"regressions"`
	ConcurrencyHazards       int      `json:"concurrency_hazards"`
	TotalDefects             int      `json:"total_defects"`
	TotalTests               int      `json:"total_tests"`
	PassedTests              int      `json:"passed_tests"`
	TestPassRatePercent      float64  `json:"test_pass_rate_percent"`
	InputTokens              int      `json:"input_tokens"`
	OutputTokens             int      `json:"output_tokens"`
	TotalTokens              int      `json:"total_tokens"`
	ExecutionDurationMs      int64    `json:"execution_duration_ms"`
	ReviewerPreMergeDefects  int      `json:"reviewer_premerge_defects"`
	ReviewerArrestedDefects  int      `json:"reviewer_arrested_defects"`
	ReviewerArrestRatePercent float64 `json:"reviewer_arrest_rate_percent"`
	FailureModes             []string `json:"failure_modes,omitempty"`
}

// TaskComparison encapsulates the comparative performance of a task between Condition A and Condition B.
type TaskComparison struct {
	TaskID                     string           `json:"task_id"`
	Title                      string           `json:"title"`
	Subsystem                  string           `json:"subsystem"`
	TargetVulnerability        string           `json:"target_vulnerability"`
	ConditionA                 ConditionMetrics `json:"condition_a"`
	ConditionB                 ConditionMetrics `json:"condition_b"`
	DriftReductionPercent      float64          `json:"drift_reduction_percent"`
	DefectReductionPercent     float64          `json:"defect_reduction_percent"`
	PassRateImprovementPercent float64          `json:"pass_rate_improvement_percent"`
	TokenExpansionRatio        float64          `json:"token_expansion_ratio"`
}

// AggregateStats holds the descriptive statistical aggregates for both conditions across all 10 tasks.
type AggregateStats struct {
	ConditionAMeanDrift        float64 `json:"condition_a_mean_drift_percent"`
	ConditionBMeanDrift        float64 `json:"condition_b_mean_drift_percent"`
	ConditionAStdDevDrift      float64 `json:"condition_a_stddev_drift_percent"`
	ConditionBStdDevDrift      float64 `json:"condition_b_stddev_drift_percent"`
	ConditionAMeanDefects      float64 `json:"condition_a_mean_defects"`
	ConditionBMeanDefects      float64 `json:"condition_b_mean_defects"`
	ConditionAStdDevDefects    float64 `json:"condition_a_stddev_defects"`
	ConditionBStdDevDefects    float64 `json:"condition_b_stddev_defects"`
	ConditionAMeanPassRate     float64 `json:"condition_a_mean_test_pass_rate_percent"`
	ConditionBMeanPassRate     float64 `json:"condition_b_mean_test_pass_rate_percent"`
	ConditionAStdDevPassRate   float64 `json:"condition_a_stddev_test_pass_rate_percent"`
	ConditionBStdDevPassRate   float64 `json:"condition_b_stddev_test_pass_rate_percent"`
	ConditionATotalTokens      int     `json:"condition_a_total_tokens"`
	ConditionBTotalTokens      int     `json:"condition_b_total_tokens"`
	OverallCostRatio           float64 `json:"overall_cost_ratio"`
	ConditionAMeanArrestRate   float64 `json:"condition_a_mean_reviewer_arrest_rate_percent"`
	ConditionBMeanArrestRate   float64 `json:"condition_b_mean_reviewer_arrest_rate_percent"`
}

// AblationReport represents the complete output of the ablation experiment suite.
type AblationReport struct {
	Title               string           `json:"title"`
	GeneratedAt         string           `json:"generated_at"`
	TaskCohort          []TaskComparison `json:"task_cohort"`
	Aggregate           AggregateStats   `json:"aggregate"`
	ExecutiveSummary    string           `json:"executive_summary"`
	QualitativeFindings []string         `json:"qualitative_findings"`
}
