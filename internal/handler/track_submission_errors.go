package handler

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"strings"
)

type submissionErrorText struct{ code, en, zh string }

var submissionErrors = map[string]submissionErrorText{
	"submission description length must be between 20 and 500": {"submission_description_length", "Route description must be between 20 and 500 characters.", "路线说明需要填写 20～500 个字符。"},
	"submission title length must be between 4 and 40":         {"submission_title_length", "Route title must be between 4 and 40 characters.", "路线标题需要填写 4～40 个字符。"},
	"transport_description length must be between 10 and 500":  {"submission_transport_length", "Transport notes must be between 10 and 500 characters.", "交通说明需要填写 10～500 个字符。"},
	"invalid difficulty":                               {"submission_difficulty", "Please select a valid difficulty.", "请选择有效的路线难度。"},
	"invalid risk_level":                               {"submission_risk", "Please select a valid risk level.", "请选择有效的风险等级。"},
	"invalid suitable_months":                          {"submission_months", "Please select valid suitable months.", "请选择有效的适宜月份。"},
	"at least one suitable month is required":          {"submission_months_required", "Please select at least one suitable month.", "请至少选择一个适宜月份。"},
	"invalid surface_types":                            {"submission_surfaces", "Please select valid surface types.", "请重新选择有效的路面类型。"},
	"surface_types is required":                        {"submission_surfaces_required", "Please select at least one surface type.", "请至少选择一种路面类型。"},
	"invalid transport_modes":                          {"submission_transport", "Please select valid transport modes.", "请重新选择有效的交通方式。"},
	"transport_modes is required":                      {"submission_transport_required", "Please select at least one transport mode.", "请至少选择一种交通方式。"},
	"submission image count must not exceed 9":         {"submission_images_count", "You can submit up to 9 images.", "投稿图片最多为 9 张。"},
	"invalid submission image oss_url":                 {"submission_image_url", "Invalid image. Please remove it and add it again.", "图片资料无效，请移除后重新添加。"},
	"submission image does not belong to current user": {"submission_image_owner", "This image belongs to another account. Please add it again.", "图片不属于当前账号，请重新添加。"},
	"invalid submission image":                         {"submission_image", "Invalid image details. Please remove it and add it again.", "图片资料无效，请移除后重新添加。"},
	"track is not eligible for submission":             {"submission_ineligible", "Upload the completed track and route screenshot before submitting.", "这条轨迹暂时不能投稿，请先上传已结束轨迹和路线截图。"},
	"submission state conflict":                        {"submission_conflict", "Submission status changed. Reopen this page and try again.", "投稿状态已变化，请重新打开页面后再试。"},
	"not found":                                        {"submission_not_found", "The submission or track was not found. Go back and refresh.", "投稿或轨迹不存在，请返回刷新。"},
	"forbidden":                                        {"submission_forbidden", "This account cannot perform this submission action.", "当前账号无权进行这项投稿操作。"},
	"unauthorized":                                     {"submission_unauthorized", "Please sign in again.", "登录已失效，请重新登录。"},
	"invalid payload":                                  {"submission_payload", "Invalid submission data. Please check it and try again.", "提交资料格式有误，请检查后重试。"},
	"track submission service not configured":          {"submission_unavailable", "Submission service is unavailable. Please try again later.", "投稿服务暂时不可用，请稍后重试。"},
}

func respondSubmissionError(c *app.RequestContext, status int, raw string) {
	text, ok := submissionErrors[raw]
	if !ok {
		text = submissionErrorText{"submission_failed", "Something went wrong. Please try again later.", "操作失败，请稍后重试。"}
	}
	language := string(c.Request.Header.Peek("X-Client-Language"))
	if meta := middleware.GetRequestMeta(c); meta != nil && meta.ClientLanguage != "" {
		language = meta.ClientLanguage
	}
	message := text.en
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		message = text.zh
	}
	c.JSON(status, utils.H{"error": message, "error_code": text.code})
}
