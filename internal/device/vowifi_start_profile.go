package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
	"github.com/yibaiba/hideck/internal/modem"
	"github.com/yibaiba/hideck/pkg/logger"
)

func (p *Pool) buildVoWiFiStartProfile(worker *Worker, traceID string) (identity.Profile, error) {
	if worker == nil {
		return identity.Profile{}, fmt.Errorf("worker_nil")
	}
	if worker.Backend == nil {
		return identity.Profile{}, fmt.Errorf("backend_not_available")
	}

	liveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owner, releaseOwnership, err := p.lockVoWiFiStartOwnership(worker)
	if err != nil {
		return identity.Profile{}, err
	}
	defer releaseOwnership()
	iccid, imsi, err := readVoWiFiStartIdentity(liveCtx, worker)
	if err != nil {
		return identity.Profile{}, err
	}

	status := worker.ProjectDeviceStatus()
	mcc, mnc, plmnSource := resolveVoWiFiProfileMCCMNC(liveCtx, worker, status, imsi, traceID)
	if mcc == "" || mnc == "" {
		return identity.Profile{}, fmt.Errorf("缺少 SIM 归属 MCC/MNC，无法构建 VoWiFi 启动画像: %s", imsi)
	}

	imei := strings.TrimSpace(status.IMEI)

	smsc := p.resolveVoWiFiSMSC(worker, imsi, iccid, traceID)
	if err := p.validatePolicyOwner(owner, p.policyTransitionFor(worker.ID)); err != nil {
		return identity.Profile{}, fmt.Errorf("VoWiFi 启动期间设备或 SIM 已更换: %w", err)
	}

	logger.Info("VoWiFi 启动画像将基于实时 IMSI 构建",
		"trace_id", traceID,
		"device", worker.ID,
		"source", "live_imsi",
		"plmn_source", plmnSource,
		"native_mcc", strings.TrimSpace(status.NativeMCC),
		"native_mnc", strings.TrimSpace(status.NativeMNC),
		"iccid", iccid,
		"imsi", imsi,
		"mcc", mcc,
		"mnc", mnc,
		"imei", imei)

	return buildVoWiFiRawProfile(identity.Profile{
		ICCID: iccid, IMSI: imsi, MCC: mcc, MNC: mnc, IMEI: imei, SMSC: smsc,
	}), nil
}

func (p *Pool) lockVoWiFiStartOwnership(worker *Worker) (policyOwner, func(), error) {
	transition := p.policyTransitionFor(worker.ID)
	transition.Lock()
	if p.GetWorker(worker.ID) != worker {
		transition.Unlock()
		return policyOwner{}, nil, fmt.Errorf("VoWiFi 启动设备已被移除或替换")
	}
	owner := capturePolicyOwner(worker, transition)
	if err := p.validatePolicyOwner(owner, transition); err != nil {
		transition.Unlock()
		return policyOwner{}, nil, fmt.Errorf("VoWiFi 启动设备或 SIM 状态已失效: %w", err)
	}
	return owner, transition.Unlock, nil
}

func readVoWiFiStartIdentity(ctx context.Context, worker *Worker) (string, string, error) {
	if reader, ok := worker.Backend.(liveSIMIdentitySnapshotReader); ok {
		snapshot, err := reader.ReadSIMIdentityLive(ctx)
		if err != nil {
			return "", "", fmt.Errorf("实时读取 SIM 身份快照失败: %w", err)
		}
		return validateVoWiFiStartIdentity(snapshot.ICCID, snapshot.IMSI)
	}
	reader, ok := worker.Backend.(liveSIMIdentityReader)
	if !ok {
		return "", "", fmt.Errorf("live_identity_not_supported")
	}
	imsi, err := reader.GetIMSILive(ctx)
	if err != nil {
		return "", "", fmt.Errorf("实时读取 IMSI 失败: %w", err)
	}
	iccid, err := reader.GetICCIDLive(ctx)
	if err != nil {
		return "", "", fmt.Errorf("实时读取 ICCID 失败: %w", err)
	}
	return validateVoWiFiStartIdentity(iccid, imsi)
}

func validateVoWiFiStartIdentity(iccid, imsi string) (string, string, error) {
	imsi = strings.TrimSpace(imsi)
	if imsi == "" {
		return "", "", fmt.Errorf("实时 IMSI 为空")
	}
	iccid = strings.TrimSpace(iccid)
	if iccid == "" {
		return "", "", fmt.Errorf("实时 ICCID 为空")
	}
	return iccid, imsi, nil
}

func (p *Pool) resolveVoWiFiSMSC(worker *Worker, imsi, iccid, traceID string) string {
	if worker == nil {
		return ""
	}
	cacheKey := strings.TrimSpace(imsi)
	if cacheKey == "" {
		cacheKey = strings.TrimSpace(iccid)
	}
	read := func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		value, err := worker.getSMSCWithContext(ctx)
		return strings.TrimSpace(value), err
	}
	smsc, err := read()
	if smsc == "" {
		smsc, err = read()
	}
	if smsc != "" {
		if p != nil && cacheKey != "" {
			p.smscCacheMu.Lock()
			if p.smscCache == nil {
				p.smscCache = make(map[string]string)
			}
			p.smscCache[cacheKey] = smsc
			p.smscCacheMu.Unlock()
		}
		logger.Info("VoWiFi 启动前获取 SMSC 成功", "trace_id", traceID, "device", worker.ID)
		return smsc
	}
	if p != nil && cacheKey != "" {
		p.smscCacheMu.RLock()
		cached := p.smscCache[cacheKey]
		p.smscCacheMu.RUnlock()
		if cached != "" {
			logger.Warn("VoWiFi 启动前读取 SMSC 失败，使用缓存", "trace_id", traceID, "device", worker.ID, "err", err)
			return cached
		}
	}
	if err != nil {
		logger.Warn("VoWiFi 启动前获取 SMSC 失败，将以空 SMSC 继续启动", "trace_id", traceID, "device", worker.ID, "err", err)
	} else {
		logger.Warn("VoWiFi 启动前未获取到 SMSC，将以空 SMSC 继续启动", "trace_id", traceID, "device", worker.ID)
	}
	return ""
}

func buildVoWiFiRawProfile(profile identity.Profile) identity.Profile {
	return identity.Profile{
		ICCID:     strings.TrimSpace(profile.ICCID),
		IMSI:      strings.TrimSpace(profile.IMSI),
		MCC:       strings.TrimSpace(profile.MCC),
		MNC:       strings.TrimSpace(profile.MNC),
		IMEI:      strings.TrimSpace(profile.IMEI),
		UserAgent: strings.TrimSpace(profile.UserAgent),
		SMSC:      strings.TrimSpace(profile.SMSC),
		IMSDomain: strings.TrimSpace(profile.IMSDomain),
	}
}

func resolveVoWiFiProfileMCCMNC(ctx context.Context, worker *Worker, status modem.DeviceStatus, imsi, traceID string) (mcc, mnc, source string) {
	imsi = strings.TrimSpace(imsi)
	if worker != nil && worker.Backend != nil {
		if liveMCC, liveMNC, err := worker.Backend.GetNativeMCCMNC(ctx); err == nil {
			liveMCC = strings.TrimSpace(liveMCC)
			liveMNC = strings.TrimSpace(liveMNC)
			if liveMCC != "" && liveMNC != "" {
				cacheVoWiFiProfileMCCMNC(worker, liveMCC, liveMNC)
				return liveMCC, liveMNC, "sim_home"
			}
		} else {
			logger.Debug("VoWiFi 启动前读取 SIM 归属 MCC/MNC 失败，将回退已缓存 SIM 归属信息",
				"trace_id", traceID, "device", worker.ID, "err", err)
		}
	}

	statusIMSI := strings.TrimSpace(status.IMSI)
	if statusIMSI == imsi && strings.TrimSpace(status.NativeMCC) != "" && strings.TrimSpace(status.NativeMNC) != "" {
		return strings.TrimSpace(status.NativeMCC), strings.TrimSpace(status.NativeMNC), "sim_home_cache"
	}

	return vowifiProfileMCCMNC(status)
}

func cacheVoWiFiProfileMCCMNC(worker *Worker, mcc, mnc string) {
	if worker == nil || strings.TrimSpace(mcc) == "" || strings.TrimSpace(mnc) == "" {
		return
	}
	worker.cacheMu.Lock()
	worker.state.Identity.NativeMCC = strings.TrimSpace(mcc)
	worker.state.Identity.NativeMNC = strings.TrimSpace(mnc)
	worker.cacheMu.Unlock()
}

func vowifiProfileMCCMNC(status modem.DeviceStatus) (mcc, mnc, source string) {
	mcc = strings.TrimSpace(status.NativeMCC)
	mnc = strings.TrimSpace(status.NativeMNC)
	if mcc != "" && mnc != "" {
		return mcc, mnc, "sim_home_cache"
	}
	return "", "", ""
}

func newVoWiFiSIMReadyStartupState(deviceID, dataplaneMode, networkMode string, now time.Time) runtimehost.State {
	return runtimehost.State{
		Phase:         runtimehost.PhaseSIMReady,
		DeviceID:      deviceID,
		DataplaneMode: dataplaneMode,
		NetworkMode:   strings.TrimSpace(networkMode),
		SIMReady:      true,
		LastReason:    "sim_ready",
		UpdatedAt:     now,
	}
}

func (p *Pool) BuildVoWiFiStartProfile(worker *Worker) (identity.Profile, error) {
	return p.buildVoWiFiStartProfile(worker, "")
}
