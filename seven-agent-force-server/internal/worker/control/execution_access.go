package control

import (
	"context"
	"net/http"
	"strconv"

	executionaccess "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func (c *Client) executionAccessEnvelope(attempt taskdomain.Attempt, sessionGeneration int64) map[string]any {
	body := c.attemptEnvelope(attempt.Generation, attempt.OwnerEpoch)
	body["attemptLeaseToken"] = attempt.LeaseToken
	body["adapterSessionGeneration"] = strconv.FormatInt(sessionGeneration, 10)
	return body
}

func (c *Client) ClaimNextExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64) (executionaccess.Claim, error) {
	var result executionaccess.Claim
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/claim-next"), c.executionAccessEnvelope(attempt, sessionGeneration), "", nil, &result)
	return result, err
}

func (c *Client) PendingSuspendExecutionOperations(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64) ([]executionaccess.PendingSuspendOperation, error) {
	var result []executionaccess.PendingSuspendOperation
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/pending-suspend"), c.executionAccessEnvelope(attempt, sessionGeneration), "", nil, &result)
	return result, err
}

func (c *Client) StartExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, input executionaccess.StartRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["start"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(input.OperationID)+"/start"), body, "", nil, &result)
	return result, err
}

func (c *Client) RenewExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, input executionaccess.RenewRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["renew"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(input.OperationID)+"/renew"), body, "", nil, &result)
	return result, err
}

func (c *Client) SubmitExecutionOperationResult(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, input executionaccess.ResultRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["result"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(input.OperationID)+"/result"), body, "", nil, &result)
	return result, err
}

func (c *Client) ReadExecutionOperationResultReceipt(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, input executionaccess.ResultReceiptRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["receipt"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(input.OperationID)+"/result-receipt"), body, "", nil, &result)
	return result, err
}

func (c *Client) PullExecutionOperationResults(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, limit int) ([]executionaccess.ToolResult, error) {
	var result []executionaccess.ToolResult
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["limit"] = limit
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/results/pull"), body, "", nil, &result)
	return result, err
}

func (c *Client) AckExecutionOperationResult(ctx context.Context, attempt taskdomain.Attempt, sessionGeneration int64, input executionaccess.ResultAckRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sessionGeneration)
	body["ack"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(input.OperationID)+"/result-ack"), body, "", nil, &result)
	return result, err
}

func (c *Client) ClaimColdExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64) (executionaccess.Claim, error) {
	var result executionaccess.Claim
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/claim"), body, "", nil, &result)
	return result, err
}

func (c *Client) StartColdExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64, input executionaccess.StartRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	body["start"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/start"), body, "", nil, &result)
	return result, err
}

func (c *Client) RenewColdExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64, input executionaccess.RenewRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	body["renew"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/renew"), body, "", nil, &result)
	return result, err
}

func (c *Client) SubmitColdExecutionOperationResult(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64, input executionaccess.ResultRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	body["result"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/result"), body, "", nil, &result)
	return result, err
}

func (c *Client) ReadColdExecutionOperationResultReceipt(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64, input executionaccess.ResultReceiptRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	body["receipt"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/result-receipt"), body, "", nil, &result)
	return result, err
}

func (c *Client) ReadbackColdExecutionOperation(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64) (executionaccess.ToolResult, error) {
	var result executionaccess.ToolResult
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/readback"), body, "", nil, &result)
	return result, err
}

func (c *Client) AckColdExecutionOperationResult(ctx context.Context, attempt taskdomain.Attempt, sourceSessionGeneration, operationID int64, input executionaccess.ResultAckRequest) (executionaccess.Receipt, error) {
	var result executionaccess.Receipt
	body := c.executionAccessEnvelope(attempt, sourceSessionGeneration)
	body["ack"] = input
	err := c.call(ctx, http.MethodPost, c.attemptPath(attempt.ID, "/execution-operations/"+idText(operationID)+"/cold/result-ack"), body, "", nil, &result)
	return result, err
}
