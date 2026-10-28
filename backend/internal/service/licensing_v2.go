package service

import "github.com/yunnuo88520/yunnuo-license/backend/internal/licensing"

// Resolve the current store for every request, including after first-run
// installation switches from its bootstrap database to the configured one.
func (s *Service) Licensing() *licensing.Service {
	return licensing.New(s.currentStore().LicensingRegistry(), s.cardPepper, s.dataKey).WithClock(s.now).WithReleaseStorage(s.releaseDir, licensing.StorageLimits{TotalBytes: s.resourceLimits.TotalBytes, DeveloperBytes: s.resourceLimits.DeveloperBytes, ApplicationBytes: s.resourceLimits.ProductBytes, Versions: s.resourceLimits.ProductVersions})
}
