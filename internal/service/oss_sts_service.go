package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tongyichu/track_server/internal/config"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	ossclient "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSTemporaryCredential 表示服务端向客户端返回的 OSS 临时凭证。
//
// 背景：客户端希望“直传 OSS”（而不是先把文件上传到业务服务器再转存）。
// 为了避免把长期 AK/SK 暴露给客户端，服务端会通过阿里云 STS 申请一个短时有效的临时凭证。
//
// 关键点：
// 1) 该临时凭证本质上是 STS 返回的三元组：AccessKeyId/AccessKeySecret/SecurityToken。
// 2) 临时凭证权限 = 角色(Role)权限 ∩ AssumeRole 的 Policy 权限（交集）。
// 3) 我们在 Policy 中将 Resource 限制到“用户专属目录前缀”，实现按用户隔离。
//
// 说明：Bucket/Region/Endpoint/Dir 这些字段是“客户端直传”时常见的必要上下文信息，
// 便于前端拼装上传 URL 与 object key。
type OSSTemporaryCredential struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
	Expiration      string `json:"expiration"`

	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"`
	Dir      string `json:"dir"`
}

// OSSTokenService 负责向阿里云 STS 申请“仅允许上传到指定前缀”的临时凭证。
//
// 设计取舍：
//   - 本服务只做“发放上传凭证”，不参与实际上传，也不生成 postPolicy 表单。
//   - 权限上目前最小化到 `oss:PutObject`，即“仅允许上传对象”。
//     如需支持分片上传/列举/删除等，应在 Policy 的 Action 中显式加上对应权限。
type OSSTokenService struct {
	// stsClient 是阿里云 STS SDK 客户端，使用长期 AK/SK 初始化，仅在服务端持有。
	stsClient *sts.Client

	// roleARN 是用于 AssumeRole 的 RAM 角色 ARN。
	roleARN string
	// durationSeconds 是临时凭证有效期（秒）。
	// 为减少风险，应尽量短（例如 15 分钟或 1 小时），并符合阿里云侧允许范围。
	durationSeconds int64
	// roleSessionPrefix 用于拼接 RoleSessionName（便于审计与排障）。
	roleSessionPrefix string

	// ossBucket 是 OSS Bucket 名称。
	ossBucket string
	// ossRegion/ossEndpoint 是给客户端上传使用的区域/Endpoint 信息（可选）。
	ossRegion   string
	ossEndpoint string
	// ossInternalEndpoint 仅供服务端从 OSS 拉取对象使用，走内网避免公网流量费用。
	// 若未配置，服务端下载会直接返回错误。
	ossInternalEndpoint string
	// ossUploadPrefix 控制用户目录的一级前缀，例如 "user"；最终目录为 <prefix>/<userID>/。
	ossUploadPrefix string
}

// NewOSSTokenService 创建 STS Token Service。
//
// 必填参数（建议通过环境变量注入）：
// - accessKeyID/accessKeySecret：服务端长期 AK/SK（严禁下发给客户端）
// - roleARN：用于 AssumeRole 的 RAM 角色 ARN
// - stsRegion：STS 的 region（例如 cn-hangzhou）
// - ossBucket：要允许上传的 bucket
//
// durationSeconds：临时凭证有效期；这里会做 900~3600 的钳制，避免配置过小/过大导致 AssumeRole 失败。
// roleSessionPrefix：会与 userID/timestamp 组合形成 RoleSessionName，便于云侧审计。
// ossUploadPrefix：用于生成用户隔离目录，默认 "user"。
func NewOSSTokenService(stsRegion, accessKeyID, accessKeySecret, roleARN string, durationSeconds int64, roleSessionPrefix string,
	ossBucket, ossRegion, ossEndpoint, ossInternalEndpoint, ossUploadPrefix string,
) (*OSSTokenService, error) {
	if accessKeyID == "" || accessKeySecret == "" || roleARN == "" {
		return nil, errors.New("missing ALIYUN_ACCESS_KEY_ID/ALIYUN_ACCESS_KEY_SECRET/ALIYUN_ROLE_ARN")
	}
	if stsRegion == "" {
		return nil, errors.New("missing ALIYUN_STS_REGION")
	}
	if ossBucket == "" {
		return nil, errors.New("missing OSS_BUCKET")
	}
	if durationSeconds <= 0 {
		durationSeconds = 3600
	}
	// 注意：阿里云 STS 的 AssumeRole 对 DurationSeconds 通常有范围限制。
	// 常见限制为 900~3600 秒（15 分钟到 1 小时）。这里做钳制：
	// - 小于 900：提升到 900，避免请求失败
	// - 大于 3600：降低到 3600，避免请求失败
	if durationSeconds < 900 {
		durationSeconds = 900
	}
	if durationSeconds > 3600 {
		durationSeconds = 3600
	}

	cli, err := sts.NewClientWithAccessKey(stsRegion, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}

	if roleSessionPrefix == "" {
		roleSessionPrefix = "trackapp-"
	}
	if ossUploadPrefix == "" {
		ossUploadPrefix = "user"
	}

	return &OSSTokenService{
		stsClient:           cli,
		roleARN:             roleARN,
		durationSeconds:     durationSeconds,
		roleSessionPrefix:   roleSessionPrefix,
		ossBucket:           ossBucket,
		ossRegion:           ossRegion,
		ossEndpoint:         ossEndpoint,
		ossInternalEndpoint: ossInternalEndpoint,
		ossUploadPrefix:     ossUploadPrefix,
	}, nil
}

// GetReleaseUploadCredential 为 App 安装包上传申请一份仅允许写入 release 目录的临时凭证。
//
// 与 GetUploadCredential 的区别：
//   - 这里不做“按 userID 隔离”的子目录；所有管理员上传到同一个 release 目录下；
//   - 目录形如 "release/"（若传入 subDir，则为 "release/<subDir>/"）。
//
// 使用场景：管理后台（admin）发布 Android APK 时，浏览器直接使用该凭证 PUT 到 OSS，
// 避免把几十 MB 的安装包经过 Hertz 服务端中转。
func (s *OSSTokenService) GetReleaseUploadCredential(subDir string) (*OSSTemporaryCredential, error) {
	base := "release"
	dir := base + "/"
	sub := strings.Trim(subDir, "/")
	if sub != "" {
		dir = base + "/" + sub + "/"
	}

	policyJSON, err := buildOSSPutObjectPolicy(s.ossBucket, dir)
	if err != nil {
		log.Printf("debug info failed to build release policy JSON: %v", err)
		return nil, err
	}

	roleSessionName := fmt.Sprintf("%srelease-%d", s.roleSessionPrefix, time.Now().Unix())
	if len(roleSessionName) > 64 {
		roleSessionName = roleSessionName[:64]
	}

	req := sts.CreateAssumeRoleRequest()
	req.Scheme = "https"
	req.RoleArn = s.roleARN
	req.RoleSessionName = roleSessionName
	req.DurationSeconds = requests.NewInteger(int(s.durationSeconds))
	req.Policy = policyJSON

	resp, err := s.stsClient.AssumeRole(req)
	if err != nil {
		log.Printf("debug info sts AssumeRole(release) error [req=%v]: %v", req, err)
		return nil, err
	}
	cred := resp.Credentials
	if cred.AccessKeyId == "" || cred.AccessKeySecret == "" || cred.SecurityToken == "" {
		return nil, errors.New("sts returned empty credentials")
	}

	return &OSSTemporaryCredential{
		AccessKeyID:     cred.AccessKeyId,
		AccessKeySecret: cred.AccessKeySecret,
		SecurityToken:   cred.SecurityToken,
		Expiration:      cred.Expiration,
		Bucket:          s.ossBucket,
		Region:          s.ossRegion,
		Endpoint:        s.ossEndpoint,
		Dir:             dir,
	}, nil
}

// GetUploadCredential 为指定 userID 申请一份“仅允许上传到该用户目录”的临时凭证。
//
// 目录隔离：最终 dir 形如：<OSSUploadPrefix>/<userID>/，例如 "user/123/"。
// 然后把 policy Resource 限制为：acs:oss:*:*:<bucket>/<dir>*，从而只允许写入该前缀。
//
// 安全提示：
// - 该临时凭证可被拿到的人在有效期内向该目录写入对象；服务端应确保接口受登录态保护。
// - 若希望进一步约束 object key（例如只能上传某个固定文件名），可以把 dir 细化到更具体的前缀。
func (s *OSSTokenService) GetUploadCredential(userID int64) (*OSSTemporaryCredential, error) {
	if userID <= 0 {
		return nil, invalidArg("user_id must be > 0")
	}

	// 强制生成稳定的用户目录：<prefix>/<userID>/
	// - prefix 从配置读取，但会去掉左右斜杠，避免出现 // 或空前缀
	// - 目录统一以 / 结尾，便于拼接 object key（dir + filename）
	base := strings.Trim(s.ossUploadPrefix, "/")
	if base == "" {
		base = config.DefaultOSSPathPrefix
	}
	dir := fmt.Sprintf("%s/%d/", base, GetBucketID(userID))

	policyJSON, err := buildOSSPutObjectPolicy(s.ossBucket, dir)
	if err != nil {
		log.Printf("debug info failed to build policy JSON: %v", err)
		return nil, err
	}

	// RoleSessionName 是云侧审计日志里可见的 session 标识。
	// 这里带上 userID 与时间戳，既能排障也能避免冲突。
	// 阿里云对长度存在限制（常见 64），因此超长会截断。
	roleSessionName := fmt.Sprintf("%s%d-%d", s.roleSessionPrefix, userID, time.Now().Unix())
	if len(roleSessionName) > 64 {
		roleSessionName = roleSessionName[:64]
	}

	req := sts.CreateAssumeRoleRequest()
	// 强制 https，避免明文传输。
	req.Scheme = "https"
	req.RoleArn = s.roleARN
	req.RoleSessionName = roleSessionName
	// SDK 的 Integer 类型要求 int，这里做显式转换。
	req.DurationSeconds = requests.NewInteger(int(s.durationSeconds))
	// Policy 是 JSON 字符串。STS 最终权限取 Role 权限与该 Policy 的交集。
	req.Policy = policyJSON

	resp, err := s.stsClient.AssumeRole(req)
	if err != nil {
		log.Printf("debug info sts AssumeRole error [req=%v]: %v", req, err)
		return nil, err
	}
	cred := resp.Credentials
	// Credentials 为空通常意味着 STS 侧返回异常；这里做防御性校验。
	if cred.AccessKeyId == "" || cred.AccessKeySecret == "" || cred.SecurityToken == "" {
		log.Printf("debug info sts returned empty credentials")
		return nil, errors.New("sts returned empty credentials")
	}

	return &OSSTemporaryCredential{
		AccessKeyID:     cred.AccessKeyId,
		AccessKeySecret: cred.AccessKeySecret,
		SecurityToken:   cred.SecurityToken,
		Expiration:      cred.Expiration,
		Bucket:          s.ossBucket,
		Region:          s.ossRegion,
		Endpoint:        s.ossEndpoint,
		Dir:             dir,
	}, nil
}

func GetBucketID(uid int64) int64 {
	return uid % config.OSSFileBucketSize
}

// GetReadCredential 为指定 userID 申请一份“仅允许读取”的临时凭证。
//
// 目录隔离：相比 GetUploadCredential，读权限放宽到“用户目录的上一层”，
// 最终 dir 形如：<OSSUploadPrefix>/，例如 "user/"。
// 这样客户端可以读取该前缀下任意用户的对象（例如浏览他人公开的轨迹资源）。
//
// 安全提示：
//   - 此凭证只能读取，不能写入/删除。
//   - 调用方需自行确保业务层面允许跨用户读取；若需更严格隔离，应改用 GetUploadCredential 同级的用户目录前缀。
func (s *OSSTokenService) GetReadCredential(userID int64) (*OSSTemporaryCredential, error) {
	if userID <= 0 {
		return nil, invalidArg("user_id must be > 0")
	}

	// 读权限放宽到用户目录上一层：<prefix>/
	base := strings.Trim(s.ossUploadPrefix, "/")
	if base == "" {
		base = config.DefaultOSSPathPrefix
	}
	dir := fmt.Sprintf("%s/", base)

	policyJSON, err := buildOSSReadObjectPolicy(s.ossBucket, dir)
	if err != nil {
		log.Printf("debug info failed to build read policy JSON: %v", err)
		return nil, err
	}

	roleSessionName := fmt.Sprintf("%sread-%d-%d", s.roleSessionPrefix, userID, time.Now().Unix())
	if len(roleSessionName) > 64 {
		roleSessionName = roleSessionName[:64]
	}

	req := sts.CreateAssumeRoleRequest()
	req.Scheme = "https"
	req.RoleArn = s.roleARN
	req.RoleSessionName = roleSessionName
	req.DurationSeconds = requests.NewInteger(int(s.durationSeconds))
	req.Policy = policyJSON

	resp, err := s.stsClient.AssumeRole(req)
	if err != nil {
		log.Printf("debug info sts AssumeRole(read) error [req=%v]: %v", req, err)
		return nil, err
	}
	cred := resp.Credentials
	if cred.AccessKeyId == "" || cred.AccessKeySecret == "" || cred.SecurityToken == "" {
		log.Printf("debug info sts returned empty credentials(read)")
		return nil, errors.New("sts returned empty credentials")
	}

	return &OSSTemporaryCredential{
		AccessKeyID:     cred.AccessKeyId,
		AccessKeySecret: cred.AccessKeySecret,
		SecurityToken:   cred.SecurityToken,
		Expiration:      cred.Expiration,
		Bucket:          s.ossBucket,
		Region:          s.ossRegion,
		Endpoint:        s.ossEndpoint,
		Dir:             dir,
	}, nil
}

// buildOSSReadObjectPolicy 生成仅允许读取的 Policy。
//   - Action: oss:GetObject, oss:ListObjects
//   - Resource: acs:oss:*:*:<bucket>/<dir>*  以及  acs:oss:*:*:<bucket>
//     （ListObjects 需要作用于 bucket 级资源）
func buildOSSReadObjectPolicy(bucket, dir string) (string, error) {
	objectResource := fmt.Sprintf("acs:oss:*:*:%s/%s*", bucket, dir)
	bucketResource := fmt.Sprintf("acs:oss:*:*:%s", bucket)
	policy := map[string]any{
		"Version": "1",
		"Statement": []map[string]any{
			{
				"Action":   []string{"oss:GetObject"},
				"Effect":   "Allow",
				"Resource": []string{objectResource},
			},
			{
				"Action":   []string{"oss:ListObjects"},
				"Effect":   "Allow",
				"Resource": []string{bucketResource},
			},
		},
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildOSSPutObjectPolicy 生成 AssumeRole 使用的最小权限 Policy。
//
// 目前策略：只允许对指定 bucket + 指定 dir 前缀执行 PutObject。
// - Action: oss:PutObject
// - Resource: acs:oss:*:*:<bucket>/<dir>*
//
// 说明：
// - 这里没有加 Condition（例如 IP 限制、UA 限制），仅按 Resource 做前缀隔离。
// - 若要支持分片上传（MultipartUpload），需要额外授权相关 Action。
func buildOSSPutObjectPolicy(bucket, dir string) (string, error) {
	// Resource 示例：acs:oss:*:*:examplebucket/prod/track/*
	// 我们这里 dir 结尾是 /，因此拼接成 <dir>* 后等价于允许该目录下的任意对象。
	resource := fmt.Sprintf("acs:oss:*:*:%s/%s*", bucket, dir)
	policy := map[string]any{
		"Version": "1",
		"Statement": []map[string]any{
			{
				"Action":   []string{"oss:PutObject"},
				"Effect":   "Allow",
				"Resource": []string{resource},
			},
		},
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DownloadObject 使用服务端获取的读权限临时凭证，把指定 object 下载到本地文件。
//
// 这里的 ossObjectURL 可以是以下两种形态之一：
//  1. 完整 OSS HTTP URL（含签名，如 https://<bucket>.oss-cn-beijing.aliyuncs.com/prod/track/.../a.jpg?OSSAccessKeyId=...）
//  2. 仅 object key（如 prod/track/1888/1777034875014.jpg）
//
// 函数会从 URL 中解析出 bucket/objectKey（丢弃签名参数）。
//
// 关于 endpoint：为了避免公网下行流量产生费用，服务端**强制使用配置的内网 endpoint**
// （例如 https://oss-cn-beijing-internal.aliyuncs.com）。数据库中保存的 URL 通常是
// 客户端直传/预览使用的公网域名，这里会被有意忽略。
// 部署时应把 OSS_INTERNAL_ENDPOINT 配置为 bucket 对应地域的 internal endpoint。
func (s *OSSTokenService) DownloadObject(userID int64, ossObjectURL, localPath string) error {
	if ossObjectURL == "" {
		return errors.New("ossObjectURL is empty")
	}
	bucket, _, objectKey, err := s.parseOSSObjectURL(ossObjectURL)
	if err != nil {
		log.Printf("[OSSDownload] parse_url_error user_id=%d source=%s local=%s err=%v", userID, redactURL(ossObjectURL), localPath, err)
		return err
	}
	if bucket == "" {
		bucket = s.ossBucket
	}
	// 强制走内网 endpoint：服务器与 OSS 同地域时，内网流量免费。
	// 如果没配置 OSSInternalEndpoint，下载会直接失败，避免误走公网产生费用。
	endpoint := s.ossInternalEndpoint
	if endpoint == "" {
		err := errors.New("oss internal endpoint is not configured; please set OSS_INTERNAL_ENDPOINT to the internal endpoint to avoid public traffic fee")
		log.Printf("[OSSDownload] missing_internal_endpoint user_id=%d bucket=%s object_key=%s source=%s local=%s", userID, bucket, objectKey, redactURL(ossObjectURL), localPath)
		return err
	}
	if objectKey == "" {
		err := errors.New("failed to parse oss object key")
		log.Printf("[OSSDownload] empty_object_key user_id=%d bucket=%s source=%s local=%s", userID, bucket, redactURL(ossObjectURL), localPath)
		return err
	}

	cred, err := s.GetReadCredential(userID)
	if err != nil {
		log.Printf("[OSSDownload] get_read_credential_error user_id=%d bucket=%s object_key=%s source=%s err=%v", userID, bucket, objectKey, redactURL(ossObjectURL), err)
		return err
	}

	// 注意：endpoint 必须是 bucket 所在地域的 internal endpoint（形如 https://oss-cn-beijing-internal.aliyuncs.com）。
	// SecurityToken 来自 STS 临时凭证。
	cli, err := ossclient.New(endpoint, cred.AccessKeyID, cred.AccessKeySecret, ossclient.SecurityToken(cred.SecurityToken))
	if err != nil {
		log.Printf("[OSSDownload] new_client_error user_id=%d endpoint=%s bucket=%s object_key=%s err=%v", userID, endpoint, bucket, objectKey, err)
		return err
	}
	bkt, err := cli.Bucket(bucket)
	if err != nil {
		log.Printf("[OSSDownload] bucket_error user_id=%d endpoint=%s bucket=%s object_key=%s err=%v", userID, endpoint, bucket, objectKey, err)
		return err
	}
	if err := bkt.GetObjectToFile(objectKey, localPath); err != nil {
		log.Printf("[OSSDownload] get_object_error user_id=%d endpoint=%s bucket=%s object_key=%s local=%s err=%v", userID, endpoint, bucket, objectKey, localPath, err)
		return err
	}
	return nil
}

// parseOSSObjectURL 从 OSS 对象完整 URL 中解析出 bucket/endpoint/objectKey。
// 若传入的是相对路径（不含 scheme），则视为 objectKey。
func (s *OSSTokenService) parseOSSObjectURL(raw string) (bucket, endpoint, objectKey string, err error) {
	if !strings.Contains(raw, "://") {
		// 视为 object key（可带前导 /）
		return "", "", strings.TrimLeft(raw, "/"), nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	host := u.Host
	// 形如 <bucket>.oss-cn-beijing.aliyuncs.com
	if idx := strings.Index(host, "."); idx > 0 {
		bucket = host[:idx]
		endpoint = u.Scheme + "://" + host[idx+1:]
	} else {
		endpoint = u.Scheme + "://" + host
	}
	objectKey = strings.TrimLeft(u.Path, "/")
	return bucket, endpoint, objectKey, nil
}
