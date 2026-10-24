package application

import processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"

const (
	// SDK imports are archive content. Keep the immutable release identity
	// explicit even though built-ins are now ordinary system Processors rather
	// than a separate template catalog.
	// Published 2.0.0 archive bytes retain the legacy SDK import. The Force SDK
	// preserves that entry point; a source rename must not rewrite a Release.
	builtinRuntimeSDKVersion = "2.0.0"
	OfficialDefaultProcessor = "标准代码任务处理器"
	OfficialDefaultVersion   = builtinRuntimeSDKVersion
	ReportProcessor          = "报告任务处理器"
)

type builtinAsset struct {
	Name        string
	Category    string
	Description string
	Version     string
	Manifest    processor.Manifest
	Files       []processor.SourceFile
}

// systemProcessorAssets is the complete built-in catalog. Built-ins use the
// same Draft and immutable Release records as authored Processors; there is no
// parallel Template or Template Release product layer.
func systemProcessorAssets() []builtinAsset {
	return []builtinAsset{
		standardCodeProcessor(),
		reportProcessor(),
	}
}

func baseManifest(name string) processor.Manifest {
	manifest := processor.DefaultManifest(name)
	manifest.Version = builtinRuntimeSDKVersion
	return manifest
}

func reportProcessor() builtinAsset {
	manifest := baseManifest(ReportProcessor)
	return builtinAsset{
		Name: ReportProcessor, Category: "报告", Description: "验证并声明用户可下载的报告结果", Version: builtinRuntimeSDKVersion, Manifest: manifest,
		Files: []processor.SourceFile{{Path: "processor.py", Mode: 0o644, Content: `from seven_workforce_runtime.processor import pass_result, ask_agent, workspace_path


def process(context):
    report = "SEVEN_WORKER_RESULT.md"
    result = context.process.run(["test", "-f", report], cwd=context.workspace.repo)
    if result.returncode != 0:
        return ask_agent(
            reason_code="RESULT_REPORT_MISSING",
            message="请创建 SEVEN_WORKER_RESULT.md 并说明完成结果。",
        )
    return pass_result(
        message="结果报告已准备好。",
        outputs=[workspace_path(name="result", path=report, storage="IMMEDIATE")],
    )
`}},
	}
}

func standardCodeProcessor() builtinAsset {
	manifest := baseManifest(OfficialDefaultProcessor)
	return builtinAsset{
		Name: OfficialDefaultProcessor, Category: "代码任务", Description: "验证标准代码任务结果并声明结果文件", Version: OfficialDefaultVersion, Manifest: manifest,
		Files: []processor.SourceFile{{Path: "processor.py", Mode: 0o644, Content: `from seven_workforce_runtime.processor import ask_agent, pass_result, workspace_path


def process(context):
    result = context.process.run(
        ["test", "-f", "SEVEN_WORKER_RESULT.md"],
        cwd=context.workspace.repo,
    )
    if result.returncode != 0:
        return ask_agent(
            reason_code="DEFAULT_RESULT_FILE_MISSING",
            message="请创建 SEVEN_WORKER_RESULT.md 后重新完成任务。",
        )
    return pass_result(
        message="默认验收通过。",
        outputs=[workspace_path(name="result", path="SEVEN_WORKER_RESULT.md", storage="IMMEDIATE")],
    )
`}},
	}
}
