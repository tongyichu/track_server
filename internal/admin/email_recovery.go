package admin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
	"time"
)

type recoveryMailJob struct{ username, email, notice string }
type recoveryRate struct {
	count   int
	expires time.Time
}
type emailRecovery struct {
	manager     *AccountManager
	sender      *smtpEmailSender
	jobs        chan recoveryMailJob
	completions chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wait        sync.WaitGroup
	mu          sync.Mutex
	rates       map[string]recoveryRate
}

func (m *Module) ConfigureEmailRecovery(config SMTPConfig) error {
	if m == nil || m.Auth == nil {
		return nil
	}
	sender, err := newSMTPEmailSender(config)
	if err != nil || sender == nil {
		return err
	}
	if m.Auth.manager == nil || m.Auth.manager.storage == nil {
		return errAccountStoreUnavailable
	}
	// A server-only, purpose-specific key prevents a database-only reader from
	// brute-forcing the short numeric code. Credential rotation invalidates codes.
	key := sha256.Sum256([]byte("track-admin-email-code-v1\x00" + config.Password))
	m.Auth.manager.emailCodeKey = key[:]
	ctx, cancel := context.WithCancel(context.Background())
	r := &emailRecovery{manager: m.Auth.manager, sender: sender, jobs: make(chan recoveryMailJob, 16), completions: make(chan struct{}, 2), ctx: ctx, cancel: cancel, rates: make(map[string]recoveryRate)}
	m.Auth.recovery = r
	for i := 0; i < 2; i++ {
		r.wait.Add(1)
		go r.work()
	}
	return nil
}

func (r *emailRecovery) close() { r.cancel(); r.wait.Wait() }

func (r *emailRecovery) allow(key string, limit int, window time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	value := r.rates[key]
	if !now.Before(value.expires) {
		if len(r.rates) >= 4096 {
			for key, item := range r.rates {
				if !now.Before(item.expires) {
					delete(r.rates, key)
				}
			}
			if len(r.rates) >= 4096 {
				return false
			}
		}
		value = recoveryRate{expires: now.Add(window)}
	}
	if value.count >= limit {
		return false
	}
	value.count++
	r.rates[key] = value
	return true
}

func (r *emailRecovery) queue(job recoveryMailJob) bool {
	select {
	case <-r.ctx.Done():
		return false
	default:
	}
	select {
	case r.jobs <- job:
		return true
	default:
		return false
	}
}

func (r *emailRecovery) work() {
	defer r.wait.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case job := <-r.jobs:
			ctx, cancel := context.WithTimeout(r.ctx, 18*time.Second)
			if job.notice != "" {
				if err := r.sender.send(ctx, job.email, "Track 后台账号安全通知", fmt.Sprintf("后台账号：%s\n\n%s\n\n如非本人操作，请立即联系后台负责人核实。此邮件不包含密码。", job.username, job.notice)); err != nil {
					log.Print("admin account security notification delivery failed")
				}
			} else {
				challenge, code, err := r.manager.startPasswordRecovery(ctx, job.username, job.email)
				if err == nil && challenge != nil {
					err = r.deliverCode(ctx, job.username, emailRecoveryPurpose, challenge, code)
				}
				if err != nil {
					log.Print("admin password recovery request could not be delivered")
				}
			}
			cancel()
		}
	}
}

func (r *emailRecovery) deliverCode(ctx context.Context, username, purpose string, challenge *emailChallenge, code string) error {
	action := "验证新的找回邮箱"
	if purpose == emailChangePurpose {
		action = "验证原邮箱以更换找回邮箱"
	}
	if purpose == emailRecoveryPurpose {
		action = "重新设置后台登录密码"
	}
	body := fmt.Sprintf("后台账号：%s\n\n你正在%s，验证码为：\n\n%s\n\n请回到你正在使用的 Track 后台填写验证码。验证码 10 分钟内有效，仅可使用一次；请勿转发给他人。此验证码不能直接用于登录。\n\n如非本人操作，请忽略本邮件。邮件中不会提供或索要你的登录密码。", username, action, code)
	if err := r.sender.send(ctx, challenge.Email, "Track 后台邮箱验证码", body); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		r.manager.discardEmailChallenge(cleanup, username, purpose, challenge.Hash)
		return accountFailure(503, "邮件暂未发送成功，请稍后重试")
	}
	return nil
}

func (r *emailRecovery) notify(username, email, message string) {
	if email != "" && !r.queue(recoveryMailJob{username: username, email: email, notice: message}) {
		log.Print("admin account security notification queue unavailable")
	}
}
