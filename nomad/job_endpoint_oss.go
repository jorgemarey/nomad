// +build !ent

package nomad

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/nomad/structs"
	vapi "github.com/hashicorp/vault/api"
)

// enforceSubmitJob is used to check any Sentinel policies for the submit-job scope
func (j *Job) enforceSubmitJob(override bool, job *structs.Job) (error, error) {
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"job": job,
		}
	}
	return j.srv.enforceScope(override, structs.SentinelScopeSubmitJob, dataCB)
}

// multiregionRegister is used to send a job across multiple regions
func (j *Job) multiregionRegister(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse, newVersion uint64) (bool, error) {
	// only run this if the job is multiregion and the region provided is the global (that signals us as the runner region)

	if args.Job.IsMultiregion() && args.Job.Region == api.GlobalRegion {
		regionIndex := make(map[string]uint64)
		// query other regions for they version of the job
		for _, region := range args.Job.Multiregion.Regions {
			if region.Name != j.srv.Region() {
				var out structs.SingleJobResponse
				req := &structs.JobSpecificRequest{
					JobID: args.Job.ID,
					QueryOptions: structs.QueryOptions{
						Region:    region.Name,
						Namespace: args.Job.Namespace,
						AuthToken: args.AuthToken,
					},
				}
				if err := j.GetJob(req, &out); err != nil {
					// TODO: what to to with error
					return false, err
				}
				if out.Job != nil {
					// use highest version of the job
					if out.Job.Version >= newVersion {
						newVersion = out.Job.Version + 1
					}
					regionIndex[region.Name] = out.Job.JobModifyIndex
				}
			}
		}
		// use highest version of all the jobs
		args.Job.Version = newVersion

		// register the job in the other regions
		for _, region := range args.Job.Multiregion.Regions {
			if region.Name != j.srv.Region() {
				// interpolate job
				jobCopy := args.Job.Copy()
				j.interpolateMultiregionJobFields(jobCopy, region.Name)

				req := &structs.JobRegisterRequest{
					Job: jobCopy,
					WriteRequest: structs.WriteRequest{
						Region:    region.Name,
						Namespace: jobCopy.Namespace,
						AuthToken: args.AuthToken,
					},
					EnforceIndex:   true,
					JobModifyIndex: regionIndex[region.Name],
				}
				var out structs.JobRegisterResponse
				if err := j.Register(req, &out); err != nil {
					return false, err
				}
			}
		}
		err := j.interpolateMultiregionJobFields(args.Job, j.srv.Region())
		return true, err
	}
	return false, nil
}

// multiregionStart is used to kick-off a deployment across multiple regions
func (j *Job) multiregionStart(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse) error {
	// check every region has a deployment pending in that version

	deploymentsID := make(map[string]string)
	for _, region := range args.Job.Multiregion.Regions {
		req := &structs.JobSpecificRequest{
			JobID: args.Job.ID,
			All:   true,
			QueryOptions: structs.QueryOptions{
				Region:    region.Name,
				Namespace: args.Job.Namespace,
				AuthToken: args.AuthToken,
			},
		}

		var err error
		for i := 0; i < 5; i++ { // try severak times until we find the deployment
			var out structs.DeploymentListResponse
			err = j.Deployments(req, &out)
			for _, deployment := range out.Deployments {
				if deployment.JobVersion == args.Job.Version {
					if deployment.Status == structs.DeploymentStatusPending {
						deploymentsID[region.Name] = deployment.ID
						break
					}
					err = fmt.Errorf("Deployment for job %s in version %d at region %s is not pending. It's %s", args.Job.ID, args.Job.Version, region.Name, deployment.Status)
				}
				err = fmt.Errorf("No deployment found for job %s in version %d at region %s", args.Job.ID, args.Job.Version, region.Name)
			}
			if _, ok := deploymentsID[region.Name]; !ok {
				err = fmt.Errorf("No deployment found for job %s at region %s", args.Job.ID, region.Name)
			}
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			return err
		}
	}

	// make a deployment.Run to max_parallel regions
	maxParallel := len(args.Job.Multiregion.Regions)
	if args.Job.Multiregion.Strategy != nil && args.Job.Multiregion.Strategy.MaxParallel != 0 {
		maxParallel = args.Job.Multiregion.Strategy.MaxParallel
	}
	for i, region := range args.Job.Multiregion.Regions {
		if maxParallel <= i {
			break
		}

		req := &structs.DeploymentRunRequest{
			DeploymentID: deploymentsID[region.Name],
			WriteRequest: structs.WriteRequest{
				Region:    region.Name,
				Namespace: args.Job.Namespace,
				AuthToken: args.AuthToken,
			},
		}
		var out structs.DeploymentUpdateResponse
		if err := j.srv.staticEndpoints.Deployment.Run(req, &out); err != nil {
			return err
		}
	}
	return nil
}

// multiregionDrop is used to deregister regions from a previous version of the
// job that are no longer in use
func (j *Job) multiregionDrop(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse) error {
	// this should remove the job from those regions that has it running but were removed from the multiregion stanza
	return nil
}

// multiregionStop is used to fan-out Job.Deregister RPCs to all regions if
// the global flag is passed to Job.Deregister
func (j *Job) multiregionStop(job *structs.Job, args *structs.JobDeregisterRequest, reply *structs.JobDeregisterResponse) error {
	// TODO
	// THINK: deregister only those regions with the job in the same version as us?
	return nil
}

// interpolateMultiregionFields interpolates a job for a specific region
func (j *Job) interpolateMultiregionFields(args *structs.JobPlanRequest) error {
	return j.interpolateMultiregionJobFields(args.Job, args.Region)
}

func (j *Job) interpolateMultiregionJobFields(job *structs.Job, region string) error {
	var regionData *structs.MultiregionRegion
	if job.IsMultiregion() {
		for _, r := range job.Multiregion.Regions {
			if r.Name == region {
				regionData = r
				break
			}
		}
	}
	if regionData != nil {
		for _, group := range job.TaskGroups {
			if group.Count == 0 {
				group.Count = regionData.Count
			}
		}
		for k, v := range regionData.Meta {
			job.Meta[k] = v
		}
		if regionData.Datacenters != nil && len(regionData.Datacenters) > 0 {
			job.Datacenters = regionData.Datacenters
		}
		job.Region = regionData.Name
	}
	return nil
}

// multiVaultNamespaceValidation provides a convience check to ensure
// multiple vault namespaces were not requested, this returns an early friendly
// error before job registry and further feature checks.
func (j *Job) multiVaultNamespaceValidation(
	policies map[string]map[string]*structs.Vault,
	s *vapi.Secret,
) error {
	requestedNamespaces := structs.VaultNamespaceSet(policies)
	if len(requestedNamespaces) > 0 {
		return fmt.Errorf("multiple vault namespaces requires Nomad Enterprise, Namespaces: %s", strings.Join(requestedNamespaces, ", "))
	}
	return nil
}

// multiregion information:

// Deployments should wait until kicked off by Job.Register so that we can
// assert that all regions have a scheduled deployment before starting any
// region.  https://github.com/hashicorp/nomad/pull/8433

// - single RPC between regions to deploy (hand off) // they tried to minimize calls https://youtu.be/tw9xeSBe7HI?t=2651

// MRD
// - pending -> running -> blocked -> successful
// - la región local será la encargada de registrar el job en el resto de regiones y hacer la validación correspondiente
// - los valores se interpolan a partir de los datos existentes en el multiregion stanza
// - query all the other regions for the existing version of the job and the currect check index (use the highest version of the job)
// - forward the interpolated version of the job to each region using that region checkindex (make sure there're no concurrent updates)
// - wait until we submitted the job to every other region
// - check that every region has a deployment in it's pending state  // do you have a pending deployment for job X version Y?
// - once the deployment is ok is going to query the status for the other regions
// - then the running region is going to find it's place in the order list and ot's going to rpc to one or more other regions such as at most max_paralel regions are in the running state
// - once the rpcs are done it's going to the blocked state
// - the last region it's going to the unblocking state and send a uncking rpc to all the other regions

// - if something fails we send a deployment.fail rpc to the regions

// - if we find the deployment is cancelled on some region we will cancel the deployment (deployment.cancel)

// - when max paralel is more than 1:
//  - the region that finishes first is going to check the status of all the other regions
//  - see if there's another region still running so only send the run to at most max_parallel - running

// - interpolation  -> task -> group -> region -> job

// - failure -> default is fail next regions
