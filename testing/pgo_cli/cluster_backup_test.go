package pgo_cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/crunchydata/postgres-operator/testing/kubeapi"
	"github.com/stretchr/testify/require"
)

// TC60 ✓
// TC122 ✓
// TC130 ✓
func TestClusterBackup(t *testing.T) {
	t.Parallel()

	requireStanzaExists := func(t *testing.T, namespace, cluster string, timeout time.Duration) {
		t.Helper()

		ready := func() bool {
			output, err := pgo("show", "backup", cluster, "-n", namespace).Exec(t)
			require.NoError(t, err)
			return strings.Contains(output, "status: ok")
		}

		if !ready() {
			requireWaitFor(t, ready, timeout, time.Second,
				"timeout waiting for stanza of %q in %q", cluster, namespace)
		}
	}

	withNamespace(t, func(namespace func() string) {
		withCluster(t, namespace, func(cluster func() string) {
			t.Run("show backup", func(t *testing.T) {
				t.Run("shows something", func(t *testing.T) {
					requireClusterReady(t, namespace(), cluster(), time.Minute)

					output, err := pgo("show", "backup", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.NotEmpty(t, output)
				})
			})

			t.Run("backup", func(t *testing.T) {
				t.Run("creates an incremental backup", func(t *testing.T) {
					requireClusterReady(t, namespace(), cluster(), time.Minute)
					requireStanzaExists(t, namespace(), cluster(), 2*time.Minute)

					// BUG(cbandy): cannot create a backup too soon.
					waitFor(t, func() bool { return false }, 5*time.Second, time.Second)

					output, err := pgo("backup", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "created")

					exists := func() bool {
						output, err := pgo("show", "backup", cluster(), "-n", namespace()).Exec(t)
						require.NoError(t, err)
						return strings.Contains(output, "incr backup")
					}
					requireWaitFor(t, exists, time.Minute, time.Second,
						"timeout waiting for backup of %q in %q", cluster(), namespace())
				})

				t.Run("accepts options", func(t *testing.T) {
					requireClusterReady(t, namespace(), cluster(), time.Minute)
					requireStanzaExists(t, namespace(), cluster(), 2*time.Minute)

					// BUG(cbandy): cannot create a backup too soon.
					waitFor(t, func() bool { return false }, 5*time.Second, time.Second)

					output, err := pgo("backup", cluster(), "-n", namespace(),
						"--backup-opts=--type=diff",
					).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "created")

					exists := func() bool {
						output, err := pgo("show", "backup", cluster(), "-n", namespace()).Exec(t)
						require.NoError(t, err)
						return strings.Contains(output, "diff backup")
					}
					requireWaitFor(t, exists, time.Minute, time.Second,
						"timeout waiting for backup of %q in %q", cluster(), namespace())
				})
			})

			t.Run("create schedule", func(t *testing.T) {
				t.Run("creates a backup", func(t *testing.T) {
					t.Skip("BUG: scheduler does not handle namespaces updated after creation")

					output, err := pgo("create", "schedule", "--selector=name="+cluster(), "-n", namespace(),
						"--schedule-type=pgbackrest", "--schedule=* * * * *", "--pgbackrest-backup-type=full",
					).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "created")

					output, err = pgo("show", "schedule", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "pgbackrest-full")

					requireClusterReady(t, namespace(), cluster(), time.Minute)
					requireStanzaExists(t, namespace(), cluster(), 2*time.Minute)

					output, err = pgo("show", "backup", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					before := strings.Count(output, "full backup")

					more := func() bool {
						output, err := pgo("show", "backup", cluster(), "-n", namespace()).Exec(t)
						require.NoError(t, err)
						return strings.Count(output, "full backup") > before
					}
					requireWaitFor(t, more, 75*time.Second, time.Second,
						"timeout waiting for backup to execute on %q in %q", cluster(), namespace())
				})
			})

			t.Run("delete schedule", func(t *testing.T) {
				requireSchedule := func(t *testing.T, kind string) {
					_, err := pgo("create", "schedule", "--selector=name="+cluster(), "-n", namespace(),
						"--schedule-type=pgbackrest", "--schedule=* * * * *", "--pgbackrest-backup-type="+kind,
					).Exec(t)
					require.NoError(t, err)
				}

				t.Run("removes all schedules", func(t *testing.T) {
					requireSchedule(t, "diff")
					requireSchedule(t, "full")

					output, err := pgo("delete", "schedule", cluster(), "--no-prompt", "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "deleted")
					require.Contains(t, output, "pgbackrest-diff")
					require.Contains(t, output, "pgbackrest-full")

					output, err = pgo("show", "schedule", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.NotContains(t, output, "pgbackrest-diff")
					require.NotContains(t, output, "pgbackrest-full")
				})

				t.Run("accepts schedule name", func(t *testing.T) {
					requireSchedule(t, "diff")
					requireSchedule(t, "full")

					output, err := pgo("delete", "schedule", "-n", namespace(),
						"--schedule-name="+cluster()+"-pgbackrest-diff", "--no-prompt",
					).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "deleted")
					require.Contains(t, output, "pgbackrest-diff")
					require.NotContains(t, output, "pgbackrest-full")

					output, err = pgo("show", "schedule", cluster(), "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.NotContains(t, output, "pgbackrest-diff")
					require.Contains(t, output, "pgbackrest-full")
				})
			})
		})

		t.Run("restore", func(t *testing.T) {
			t.Run("replaces the cluster", func(t *testing.T) {
				t.Parallel()
				withCluster(t, namespace, func(cluster func() string) {
					requireClusterReady(t, namespace(), cluster(), time.Minute)
					requireStanzaExists(t, namespace(), cluster(), 2*time.Minute)

					before := clusterPVCs(t, namespace(), cluster())
					require.NotEmpty(t, before, "expected volumes to exist")

					output, err := pgo("restore", cluster(), "--no-prompt", "-n", namespace()).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, "performed")

					more := func() bool {
						after := clusterPVCs(t, namespace(), cluster())
						for _, pvc := range after {
							if !kubeapi.IsPVCBound(pvc) {
								return false
							}
						}
						return len(after) > len(before)
					}
					requireWaitFor(t, more, 2*time.Minute, time.Second,
						"timeout waiting for restore of %q in %q", cluster(), namespace())

					requireClusterReady(t, namespace(), cluster(), time.Minute)
				})
			})

			t.Run("accepts point-in-time options", func(t *testing.T) {
				t.Parallel()
				withCluster(t, namespace, func(cluster func() string) {
					requireClusterReady(t, namespace(), cluster(), time.Minute)
					requireStanzaExists(t, namespace(), cluster(), 2*time.Minute)

					// data that will need to be restored
					_, stderr := clusterPSQL(t, namespace(), cluster(),
						`CREATE TABLE important (data) AS VALUES ('treasure')`)
					require.Empty(t, stderr)

					// point to at which to restore
					recoveryObjective, stderr := clusterPSQL(t, namespace(), cluster(), `
						\set QUIET yes
						\pset format unaligned
						\pset tuples_only yes
						SELECT clock_timestamp()`)
					recoveryObjective = strings.TrimSpace(recoveryObjective)
					require.Empty(t, stderr)

					// a reason to restore followed by a WAL flush
					_, stderr = clusterPSQL(t, namespace(), cluster(), `
						DROP TABLE important;
						DO $$ BEGIN IF current_setting('server_version_num')::int > 100000
							THEN PERFORM pg_switch_wal();
							ELSE PERFORM pg_switch_xlog();
						END IF; END $$`)
					require.Empty(t, stderr)

					output, err := pgo("restore", cluster(), "-n", namespace(),
						"--backup-opts=--type=time --target-action=promote",
						"--pitr-target="+recoveryObjective, "--no-prompt",
					).Exec(t)
					require.NoError(t, err)
					require.Contains(t, output, recoveryObjective)

					restored := func() bool {
						pods, err := TestContext.Kubernetes.ListPods(
							namespace(), map[string]string{
								"pg-cluster":      cluster(),
								"pgo-pg-database": "true",
							})

						if err != nil || len(pods) == 0 {
							return false
						}

						stdout, stderr, err := TestContext.Kubernetes.PodExec(
							pods[0].Namespace, pods[0].Name,
							strings.NewReader(`TABLE important`),
							"psql", "-U", "postgres", "-f-")

						return err == nil && len(stderr) == 0 &&
							strings.Contains(stdout, "(1 row)")
					}
					requireWaitFor(t, restored, 2*time.Minute, time.Second,
						"timeout waiting for restore of %q in %q", cluster(), namespace())

					requireClusterReady(t, namespace(), cluster(), time.Minute)
				})
			})
		})
	})
}
