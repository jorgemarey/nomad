//go:build !ent
// +build !ent

package deploymentwatcher

import (
	"github.com/hashicorp/nomad/nomad/structs"
)

// DeploymentRPC and JobRPC hold methods for interacting with peer regions
// in enterprise edition.
type DeploymentRPC interface {
	Run(args *structs.DeploymentRunRequest, reply *structs.DeploymentUpdateResponse) error
	Fail(args *structs.DeploymentFailRequest, reply *structs.DeploymentUpdateResponse) error
	Unblock(args *structs.DeploymentUnblockRequest, reply *structs.DeploymentUpdateResponse) error
	Cancel(args *structs.DeploymentCancelRequest, reply *structs.DeploymentUpdateResponse) error
}
type JobRPC interface {
	GetJob(args *structs.JobSpecificRequest, reply *structs.SingleJobResponse) error
	Deployments(args *structs.JobSpecificRequest, reply *structs.DeploymentListResponse) error
}

func (w *deploymentWatcher) nextRegion(status string) error {
	//1) check every other region deployment status
	//2) find my place in the ordered list of regions and do .run on max_paralel

	// var token string
	// w.state.
	// ACLTokenByAccessorID(nil, w.j.NomadTokenID)

	// DeploymentStatusRunning    = "running"
	// DeploymentStatusPaused     = "paused"
	// DeploymentStatusFailed     = "failed"
	// DeploymentStatusSuccessful = "successful"
	// DeploymentStatusCancelled  = "cancelled"
	// DeploymentStatusPending    = "pending"
	// DeploymentStatusBlocked    = "blocked"
	// DeploymentStatusUnblocking = "unblocking"

	// for _, r := range w.j.Multiregion.Regions {
	// 	req := &structs.JobSpecificRequest{
	// 		JobID: w.d.JobID,
	// 		QueryOptions: structs.QueryOptions{
	// 			AuthToken: token,
	// 			Namespace: w.j.Namespace,
	// 			Region:    r.Name,
	// 		},
	// 	}
	// 	var resp structs.DeploymentListResponse
	// 	if err := w.JobRPC.Deployments(req, &resp); err != nil {

	// 	}

	// 	// for _, dep := range resp.Deployments {
	// 	// 	if dep.JobVersion == d.JobVersion {
	// 	// 		// return dep, nil
	// 	// 	}
	// 	// }
	// }

	// RunDeployment

	// If i'm the last region and everything went fine, unblock every other region in order
	// w.Unblock()

	// if not, get following regions deployments and on those pending fail
	// w.FailDeployment() ??? is this done by the deploymentwatcher already?

	// if a new version of the job appears we need to cancel it

	return nil
}

// RunDeployment is used to run a pending multiregion deployment.  In
// single-region deployments, the pending state is unused.
func (w *deploymentWatcher) RunDeployment(req *structs.DeploymentRunRequest, resp *structs.DeploymentUpdateResponse) error {
	// TODO: check state before upsert?
	status, desc := structs.DeploymentStatusRunning, structs.DeploymentStatusDescriptionRunning
	update := w.getDeploymentStatusUpdate(status, desc)
	eval := w.getEval()

	i, err := w.upsertDeploymentStatusUpdate(update, eval, nil)
	if err != nil {
		return err
	}

	// Build the response
	resp.EvalID = eval.ID
	resp.EvalCreateIndex = i
	resp.DeploymentModifyIndex = i
	resp.Index = i
	return nil
}

// UnblockDeployment is used to unblock a multiregion deployment.  In
// single-region deployments, the blocked state is unused.
func (w *deploymentWatcher) UnblockDeployment(req *structs.DeploymentUnblockRequest, resp *structs.DeploymentUpdateResponse) error {
	return nil
}

// CancelDeployment is used to cancel a multiregion deployment.  In
// single-region deployments, the deploymentwatcher has sole responsibility to
// cancel deployments so this RPC is never used.
func (w *deploymentWatcher) CancelDeployment(req *structs.DeploymentCancelRequest, resp *structs.DeploymentUpdateResponse) error {
	return nil
}
