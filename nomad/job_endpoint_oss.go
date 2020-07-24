// +build !ent

package nomad

import "github.com/hashicorp/nomad/nomad/structs"

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
func (j *Job) multiregionRegister(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse, existingVersion uint64) (bool, error) {
	return false, nil
}

// multiregionStart is used to kick-off a deployment across multiple regions
func (j *Job) multiregionStart(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse) error {
	return nil
}

// interpolateMultiregionFields interpolates a job for a specific region
func (j *Job) interpolateMultiregionFields(args *structs.JobPlanRequest) error {
	return nil
}
