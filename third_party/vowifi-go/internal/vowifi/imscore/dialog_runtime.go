package imscore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsendpoint"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const dialogMaxForwards = 70

func (s *Service) dialogs() *dialogRegistry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.dialogRegistry == nil {
		s.dialogRegistry = newDialogRegistry()
	}
	dialogs := s.dialogRegistry
	s.mu.Unlock()
	return dialogs
}

// NextCSeq returns the next endpoint-wide SIP sequence number.
func (s *Service) NextCSeq() uint32 {
	if s == nil {
		return 0
	}
	return s.endpointCSeq.Add(1)
}

// CloseDialog closes and removes a v1.5.5 dialog handle.
func (s *Service) CloseDialog(
	ctx context.Context,
	_ string,
	dialog imsendpoint.DialogHandle,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	handle, err := concreteDialogHandleForClose(dialog)
	if err != nil {
		return err
	}
	return s.closeDialogHandle(handle)
}

func concreteDialogHandleForClose(dialog imsendpoint.DialogHandle) (*imscoreDialogHandle, error) {
	handle, ok := dialog.(*imscoreDialogHandle)
	if !ok || handle == nil {
		return nil, errors.New("dialog handle 类型无效")
	}
	return handle, nil
}

// CloseDialogForCurrentDevice retains the additive device-implicit API.
func (s *Service) CloseDialogForCurrentDevice(
	ctx context.Context,
	dialog imsendpoint.DialogHandle,
) error {
	return s.CloseDialog(ctx, s.DeviceID(), dialog)
}

func (s *Service) closeDialogHandle(handle *imscoreDialogHandle) error {
	if s == nil || handle == nil {
		return errors.New("dialog handle 为空")
	}
	if dialogs := s.dialogs(); dialogs != nil {
		dialogs.delete(handle.id)
	}
	return closeDialogSessions(handle)
}

// SendDialogRequest sends a structured request using the retained dialog route.
func (s *Service) SendDialogRequest(
	ctx context.Context,
	_ string,
	dialog imsendpoint.DialogHandle,
	request *sip.Request,
	options imsendpoint.DialogRequestOptions,
) (*sip.Response, error) {
	if request == nil {
		return nil, errors.New("dialog request 为空")
	}
	if s == nil || !s.IsRegistered() {
		return nil, errors.New("dialog request 仅可在 IMS 注册成功后发送")
	}
	handle, err := concreteDialogHandle(dialog)
	if err != nil {
		return nil, err
	}
	return s.sendDialogRequestByMode(ctx, handle, request, options)
}

// SendDialogRequestForCurrentDevice retains the additive device-implicit API.
func (s *Service) SendDialogRequestForCurrentDevice(
	ctx context.Context,
	dialog imsendpoint.DialogHandle,
	request *sip.Request,
	options imsendpoint.DialogRequestOptions,
) (*sip.Response, error) {
	return s.SendDialogRequest(ctx, s.DeviceID(), dialog, request, options)
}

func concreteDialogHandle(dialog imsendpoint.DialogHandle) (*imscoreDialogHandle, error) {
	handle, ok := dialog.(*imscoreDialogHandle)
	if !ok || handle == nil {
		return nil, errors.New("dialog handle 类型无效")
	}
	if handle.client == nil && handle.server == nil {
		return nil, errors.New("dialog session 为空")
	}
	return handle, nil
}

func (s *Service) sendDialogRequestByMode(
	ctx context.Context,
	handle *imscoreDialogHandle,
	template *sip.Request,
	options imsendpoint.DialogRequestOptions,
) (*sip.Response, error) {
	ctx, cancel := dialogRequestContext(ctx, time.Duration(options.Timeout))
	defer cancel()
	serverRole := handle.server != nil
	if serverRole && template.Method == sip.BYE {
		if err := waitServerDialogConfirmed(ctx, handle); err != nil {
			return nil, err
		}
	}
	request, sender, _, err := s.prepareDialogRequest(handle, template)
	if err != nil {
		return nil, err
	}
	if request.IsInvite() {
		logDialogInviteRequest(request, handle)
	}
	if request.IsAck() {
		return nil, s.writeDialogACK(ctx, request, sender)
	}
	callbacks := sipTransactionCallbacks{}
	if options.OnResponse != nil && template.Method == sip.INVITE {
		handler := options.OnResponse
		callbacks.onProvisional = func(response *sipResponse) error {
			return callClientInviteResponseHandler(handler, response)
		}
	}
	transaction, err := s.transport.startClientTransactionWithSender(
		request.String(), callbacks, sender,
	)
	if err != nil {
		s.handleDialogRequestError(request, err)
		return nil, err
	}
	response, err := s.transport.waitClientTransaction(ctx, transaction)
	if err != nil {
		s.handleDialogRequestError(request, err)
		return nil, err
	}
	if serverRole && request.Method == sip.BYE {
		if response.StatusCode != 200 {
			return nil, sipgo.ErrDialogResponse{Res: response.parsed.Clone()}
		}
		return nil, nil
	}
	return response.parsed.Clone(), nil
}

func waitServerDialogConfirmed(ctx context.Context, handle *imscoreDialogHandle) error {
	handle.mu.Lock()
	if handle.confirmed {
		handle.mu.Unlock()
		return nil
	}
	confirmed := handle.confirmedCh
	inviteTx := handle.inviteTx
	handle.mu.Unlock()
	var transactionDone <-chan struct{}
	if inviteTx != nil {
		transactionDone = inviteTx.Done()
	}
	select {
	case <-confirmed:
		return nil
	case <-transactionDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func dialogRequestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		return context.WithTimeout(ctx, timeout)
	}
	return context.WithCancel(ctx)
}

func (s *Service) writeDialogACK(
	ctx context.Context,
	request *sip.Request,
	sender func(string) error,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if sender == nil {
		return errors.New("dialog SIP transport sender 为空")
	}
	err := sender(request.String())
	s.handleDialogRequestError(request, err)
	return err
}

func (s *Service) handleDialogRequestError(request *sip.Request, err error) {
	if err == nil {
		return
	}
	s.handleFatalTransactionError(err)
}

func (s *Service) prepareDialogRequest(
	handle *imscoreDialogHandle,
	template *sip.Request,
) (*sip.Request, func(string) error, bool, error) {
	request := stripDialogOwnedHeaders(template)
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.closed {
		return nil, nil, false, errors.New("dialog 已关闭")
	}
	applyDialogRecipient(request, handle)
	applyDialogCoreHeaders(s, request, handle)
	request.SetTransport(handle.inviteRequest.Transport())
	return request, s.dialogSenderLocked(handle), handle.server != nil, nil
}

func logDialogInviteRequest(request *sip.Request, handle *imscoreDialogHandle) {
	if request == nil {
		return
	}
	body := request.Body()
	contact := ""
	if request.Contact() != nil {
		contact = request.Contact().Value()
	}
	fields := []any{
		"ruri_host", strings.ToLower(strings.Trim(request.Recipient.Host, "[]")),
		"ruri_port", request.Recipient.Port,
		"ruri_user_kind", dialogRequestURIUserKind(request.Recipient),
		"ruri_params", strings.Join(dialogURIParamKeys(request.Recipient), ","),
		"via", dialogHeaderValue(request, "Via"),
		"via_port", sipSentByPort(dialogHeaderValue(request, "Via")),
		"route", strings.Join(dialogHeaderValues(request, "Route"), ","),
		"cseq", dialogHeaderValue(request, "CSeq"),
		"contact_has_ob", strings.Contains(strings.ToLower(contact), ";ob"),
		"contact_has_instance", strings.Contains(strings.ToLower(contact), "+sip.instance"),
		"session_expires_present", strings.TrimSpace(dialogHeaderValue(request, "Session-Expires")) != "",
		"body_bytes", len(body),
		"qos_remote", sdpCurrentQoSRemote(body),
	}
	fields = append(fields, dialogInviteIdentityFields(request, handle)...)
	logging.Info("IMS dialog INVITE outbound", fields...)
}

func dialogURIParamKeys(uri sip.Uri) []string {
	if uri.UriParams == nil {
		return nil
	}
	return append([]string(nil), uri.UriParams.Keys()...)
}

func dialogContactUserShape(user string) string {
	user = strings.ToLower(strings.TrimSpace(user))
	switch {
	case user == "":
		return "empty"
	case user == "orig" || user == "term":
		return user
	case strings.HasPrefix(user, "+"):
		return "e164"
	case isDialogDigits(user):
		return "digits"
	case len(user) >= 8:
		return "token"
	default:
		return "short"
	}
}

func dialogRequestURIUserKind(uri sip.Uri) string {
	user := strings.TrimSpace(uri.User)
	switch {
	case user == "":
		return "empty"
	case strings.HasPrefix(user, "+"):
		return "e164"
	case isDialogDigits(user) && len(user) <= 6:
		return "shortcode"
	case isDialogDigits(user):
		return "digits"
	default:
		return "other"
	}
}

func isDialogDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func dialogHeaderValue(request *sip.Request, name string) string {
	values := dialogHeaderValues(request, name)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func dialogHeaderValues(request *sip.Request, name string) []string {
	if request == nil {
		return nil
	}
	var values []string
	for _, header := range request.GetHeaders(name) {
		if value := strings.TrimSpace(header.Value()); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func logLearnedDialogTarget(response *sip.Response, dialog *imscoreDialogHandle) {
	if dialog == nil {
		return
	}
	dialog.mu.Lock()
	target := *dialog.remoteTarget.Clone()
	routes := append([]string(nil), dialog.routeSet...)
	dialog.mu.Unlock()
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	fields := []any{
		"status", status,
		"contact_host", strings.ToLower(strings.Trim(target.Host, "[]")),
		"contact_port", target.Port,
		"contact_user_kind", dialogRequestURIUserKind(target),
		"contact_user_shape", dialogContactUserShape(target.User),
		"contact_params", strings.Join(dialogURIParamKeys(target), ","),
		"route_count", len(routes),
		"route", strings.Join(routes, ","),
	}
	fields = append(fields, learnedDialogIdentityFields(response, dialog)...)
	logging.Info("IMS dialog target learned", fields...)
}

func sdpCurrentQoSRemote(body []byte) string {
	const prefix = "a=curr:qos remote "
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(line, "\r")))
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
