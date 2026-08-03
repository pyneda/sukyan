package db

import (
	"time"
)

// DeploymentPulseBucket is one time slice of deployment-wide activity: the jobs
// the workers finished in it, and the issues that were recorded in it.
type DeploymentPulseBucket struct {
	Start         time.Time   `json:"start"`
	JobsCompleted int64       `json:"jobs_completed"`
	JobsFailed    int64       `json:"jobs_failed"`
	Issues        IssuesStats `json:"issues"`
}

// DeploymentPulse is a fixed-length series covering the requested window. Empty
// slices are present with zero counts, so the caller never has to reconstruct
// the time axis.
type DeploymentPulse struct {
	Start         time.Time               `json:"start"`
	End           time.Time               `json:"end"`
	BucketSeconds int                     `json:"bucket_seconds"`
	Buckets       []DeploymentPulseBucket `json:"buckets"`
}

// GetDeploymentPulse buckets finished jobs and recorded issues over the window
// ending now. `bucketCount` slices of `bucketSeconds` each are always returned.
//
// Buckets are aligned to absolute epoch boundaries rather than to "now", so the
// series does not shift by a few seconds between two polls and make the chart
// jitter.
func (d *DatabaseConnection) GetDeploymentPulse(bucketSeconds int, bucketCount int) (*DeploymentPulse, error) {
	if bucketSeconds < 1 {
		bucketSeconds = 900
	}
	if bucketCount < 1 {
		bucketCount = 96
	}

	step := int64(bucketSeconds)
	endBucket := time.Now().Unix()/step + 1
	startBucket := endBucket - int64(bucketCount)

	start := time.Unix(startBucket*step, 0)
	end := time.Unix(endBucket*step, 0)

	buckets := make([]DeploymentPulseBucket, bucketCount)
	for i := range buckets {
		buckets[i].Start = time.Unix((startBucket+int64(i))*step, 0)
	}

	// Index by bucket number so both queries can write into the same series
	// without scanning it.
	slot := func(bucket int64) *DeploymentPulseBucket {
		i := bucket - startBucket
		if i < 0 || i >= int64(bucketCount) {
			return nil
		}
		return &buckets[i]
	}

	type jobRow struct {
		Bucket    int64
		Completed int64
		Failed    int64
	}
	var jobRows []jobRow
	if err := d.db.Model(&ScanJob{}).
		Select(
			"floor(extract(epoch from completed_at) / ?)::bigint as bucket, "+
				"count(*) filter (where status = ?) as completed, "+
				"count(*) filter (where status = ?) as failed",
			step, ScanJobStatusCompleted, ScanJobStatusFailed,
		).
		Where("completed_at >= ? and completed_at < ?", start, end).
		Group("bucket").
		Scan(&jobRows).Error; err != nil {
		return nil, err
	}
	for _, row := range jobRows {
		if b := slot(row.Bucket); b != nil {
			b.JobsCompleted = row.Completed
			b.JobsFailed = row.Failed
		}
	}

	type issueRow struct {
		Bucket   int64
		Severity severity
		Count    int64
	}
	var issueRows []issueRow
	if err := d.db.Model(&Issue{}).
		Select("floor(extract(epoch from created_at) / ?)::bigint as bucket, severity, count(*) as count", step).
		Where("created_at >= ? and created_at < ?", start, end).
		Where("false_positive = ?", false).
		Group("bucket, severity").
		Scan(&issueRows).Error; err != nil {
		return nil, err
	}
	for _, row := range issueRows {
		b := slot(row.Bucket)
		if b == nil {
			continue
		}
		switch row.Severity {
		case Critical:
			b.Issues.Critical += row.Count
		case High:
			b.Issues.High += row.Count
		case Medium:
			b.Issues.Medium += row.Count
		case Low:
			b.Issues.Low += row.Count
		case Info:
			b.Issues.Info += row.Count
		default:
			b.Issues.Unknown += row.Count
		}
	}

	return &DeploymentPulse{
		Start:         start,
		End:           end,
		BucketSeconds: bucketSeconds,
		Buckets:       buckets,
	}, nil
}

// GetLastJobCompletionByScan returns, per requested scan, when its most recent
// job finished. A scan whose jobs have never finished is absent from the map.
//
// Resolved for every scan in one grouped query: the overview asks for this for
// each listed scan, and a query per scan would put a dozen round trips on a
// five-second poll.
func (d *DatabaseConnection) GetLastJobCompletionByScan(scanIDs []uint) (map[uint]time.Time, error) {
	result := make(map[uint]time.Time, len(scanIDs))
	if len(scanIDs) == 0 {
		return result, nil
	}

	type row struct {
		ScanID      uint
		CompletedAt time.Time
	}
	var rows []row
	if err := d.db.Model(&ScanJob{}).
		Select("scan_id, max(completed_at) as completed_at").
		Where("scan_id in ? and completed_at is not null", scanIDs).
		Group("scan_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, r := range rows {
		result[r.ScanID] = r.CompletedAt
	}
	return result, nil
}
