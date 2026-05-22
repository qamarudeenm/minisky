import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Drawer, Box, Typography, TextField, Button,
  Stack, Divider, Alert, List, ListItem, ListItemText, IconButton,
  CircularProgress, Collapse, Paper, Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import { useProjectContext } from '../contexts/ProjectContext';

interface BuildStep {
  name: string;
  args?: string[];
}

interface Build {
  id: string;
  projectId: string;
  status: string;
  createTime: string;
  startTime?: string;
  finishTime?: string;
  steps: BuildStep[];
  source?: {
    repoSource: {
      repoName: string;
      branchName?: string;
    };
  };
}

interface LogEntry {
  timestamp: string;
  severity: string;
  message: string;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function CloudBuildDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [builds, setBuilds] = useState<Build[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [buildConfig, setBuildConfig] = useState('{\n  "steps": [\n    {\n      "name": "ubuntu",\n      "args": ["echo", "Hello from MiniSky Cloud Build!"]\n    }\n  ]\n}');
  const [submitting, setSubmitting] = useState(false);

  const [expandedBuild, setExpandedBuild] = useState<string | null>(null);
  const [sourceRepo, setSourceRepo] = useState('');
  const [sourceBranch, setSourceBranch] = useState('main');
  const [githubToken, setGithubToken] = useState('');

  // Per-build logs cache
  const [buildLogs, setBuildLogs] = useState<Record<string, LogEntry[]>>({});
  const [logsLoading, setLogsLoading] = useState<Record<string, boolean>>({});
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const logBottomRef = useRef<HTMLDivElement | null>(null);

  const fetchBuilds = useCallback(async () => {
    try {
      const res = await fetch(`/api/manage/cloudbuild/v1/projects/${activeProject}/builds`);
      if (!res.ok) throw new Error('Failed to fetch builds');
      const data = await res.json();
      setBuilds(data.builds || []);
    } catch (err: any) {
      setError(err.message);
    }
  }, [activeProject]);

  const fetchLogsForBuild = useCallback(async (buildId: string) => {
    setLogsLoading(prev => ({ ...prev, [buildId]: true }));
    try {
      const res = await fetch(
        `/api/manage/cloudbuild/v1/projects/${activeProject}/builds/${buildId}/logs`
      );
      if (!res.ok) throw new Error('Failed to fetch logs');
      const data = await res.json();
      setBuildLogs(prev => ({ ...prev, [buildId]: data.logs || [] }));
    } catch {
      // silently ignore log fetch errors
    } finally {
      setLogsLoading(prev => ({ ...prev, [buildId]: false }));
    }
  }, [activeProject]);

  // Auto-scroll log panel to bottom when new entries arrive
  useEffect(() => {
    if (expandedBuild && logBottomRef.current) {
      logBottomRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [buildLogs, expandedBuild]);

  // Poll builds + logs while any build is active
  useEffect(() => {
    if (!open) return;

    const tick = async () => {
      await fetchBuilds();
      if (expandedBuild) {
        await fetchLogsForBuild(expandedBuild);
      }
    };

    tick(); // immediate first fetch

    pollTimerRef.current = setInterval(tick, 2000);
    return () => {
      if (pollTimerRef.current) clearInterval(pollTimerRef.current);
    };
  }, [open, activeProject, expandedBuild, fetchBuilds, fetchLogsForBuild]);

  // When expanding a build, immediately fetch its logs
  const handleToggleExpand = async (buildId: string) => {
    if (expandedBuild === buildId) {
      setExpandedBuild(null);
    } else {
      setExpandedBuild(buildId);
      await fetchLogsForBuild(buildId);
    }
  };

  const handleSubmitBuild = async () => {
    setSubmitting(true);
    setError(null);
    try {
      let body;
      try {
        body = JSON.parse(buildConfig);
      } catch {
        throw new Error('Invalid JSON configuration');
      }

      const payload = {
        ...body,
        source: sourceRepo ? {
          repoSource: {
            repoName: sourceRepo,
            branchName: sourceBranch,
            ...(githubToken ? { githubToken } : {})
          }
        } : undefined
      };

      const res = await fetch(`/api/manage/cloudbuild/v1/projects/${activeProject}/builds`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error?.message || 'Failed to submit build');
      }

      await fetchBuilds();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'SUCCESS': return 'success';
      case 'FAILURE': return 'error';
      case 'WORKING': return 'primary';
      case 'QUEUED': return 'warning';
      default: return 'default';
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'ERROR': return '#f28b82';
      case 'WARNING': return '#fdd663';
      default: return '#81c995'; // INFO → green
    }
  };

  const isActive = (status: string) => status === 'WORKING' || status === 'QUEUED';

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 640, p: 4 }}>
        {/* Header */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h5" sx={{ fontWeight: 500 }}>Cloud Build</Typography>
          <IconButton onClick={onClose}><CloseIcon /></IconButton>
        </Box>

        <Divider sx={{ mb: 4 }} />

        {error && <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>{error}</Alert>}

        {/* Submit form */}
        <Paper variant="outlined" sx={{ mb: 6, p: 3, background: '#f8f9fa' }}>
          <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Submit New Build</Typography>
          <Box sx={{ mb: 3 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 1, color: '#5f6368', fontWeight: 600 }}>
              SOURCE REPOSITORY (OPTIONAL)
            </Typography>
            <Box sx={{ display: 'flex', gap: 2, mb: 1 }}>
              <TextField
                fullWidth size="small"
                placeholder="Repository (e.g. github.com/user/repo)"
                value={sourceRepo}
                onChange={(e) => setSourceRepo(e.target.value)}
                sx={{ backgroundColor: '#fff' }}
              />
              <TextField
                sx={{ width: 120, backgroundColor: '#fff' }} size="small"
                placeholder="Branch"
                value={sourceBranch}
                onChange={(e) => setSourceBranch(e.target.value)}
              />
            </Box>
            <TextField
              fullWidth size="small"
              type="password"
              placeholder="GitHub Token (required for private repos)"
              value={githubToken}
              onChange={(e) => setGithubToken(e.target.value)}
              sx={{ backgroundColor: '#fff' }}
              helperText={
                <span>
                  Private repo?{' '}
                  <a href="https://github.com/settings/tokens" target="_blank" rel="noreferrer"
                    style={{ color: '#1a73e8' }}>
                    Create a PAT
                  </a>{' '}with <code>repo</code> scope.
                </span>
              }
            />
          </Box>

          <Typography variant="caption" sx={{ display: 'block', mb: 1, color: '#5f6368', fontWeight: 600 }}>
            BUILD CONFIGURATION (JSON)
          </Typography>
          <TextField
            multiline rows={6} fullWidth variant="outlined"
            value={buildConfig}
            onChange={(e) => setBuildConfig(e.target.value)}
            placeholder='{"steps": [{"name": "ubuntu", "args": ["echo", "hello"]}]}'
            sx={{ mb: 2, '& .MuiInputBase-root': { fontFamily: 'monospace', fontSize: '0.85rem' } }}
          />
          <Button
            variant="contained" startIcon={<PlayArrowIcon />}
            onClick={handleSubmitBuild}
            disabled={submitting || !buildConfig}
            fullWidth
          >
            {submitting ? 'Submitting...' : 'Submit Build'}
          </Button>
        </Paper>

        {/* Build history */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
          <Typography variant="h6" sx={{ fontSize: '1.1rem' }}>
            Build History in {activeProject}
          </Typography>
          <IconButton size="small" onClick={() => { setLoading(true); fetchBuilds().finally(() => setLoading(false)); }}>
            {loading ? <CircularProgress size={16} /> : <RefreshIcon fontSize="small" />}
          </IconButton>
        </Box>

        {loading && builds.length === 0 ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : (
          <List disablePadding>
            {builds.length === 0 ? (
              <Typography variant="body2" sx={{ color: '#70757a', textAlign: 'center', py: 4 }}>
                No builds found in this project.
              </Typography>
            ) : (
              builds
                .sort((a, b) => b.createTime.localeCompare(a.createTime))
                .map((b) => {
                  const isExpanded = expandedBuild === b.id;
                  const logs = buildLogs[b.id] || [];
                  const fetchingLogs = logsLoading[b.id];

                  return (
                    <Box key={b.id} sx={{ mb: 1, borderBottom: '1px solid #eee' }}>
                      <ListItem sx={{ px: 1 }}>
                        <IconButton size="small" onClick={() => handleToggleExpand(b.id)} sx={{ mr: 1 }}>
                          {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                        </IconButton>
                        <ListItemText
                          primary={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                              <Typography sx={{ fontWeight: 600, fontSize: '0.9rem' }}>{b.id}</Typography>
                              {isActive(b.status) && <CircularProgress size={12} thickness={5} />}
                            </Box>
                          }
                          secondary={
                            <Box>
                              <Typography variant="caption" sx={{ display: 'block' }}>
                                Created: {new Date(b.createTime).toLocaleString()}
                              </Typography>
                              {b.source && (
                                <Typography variant="caption" sx={{ color: '#1a73e8', fontWeight: 600 }}>
                                  Source: {b.source.repoSource.repoName} ({b.source.repoSource.branchName})
                                </Typography>
                              )}
                            </Box>
                          }
                        />
                        <Chip
                          size="small" label={b.status}
                          color={getStatusColor(b.status) as any}
                          variant="filled"
                          sx={{ fontWeight: 600, fontSize: '0.7rem' }}
                        />
                      </ListItem>

                      <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                        <Box sx={{ pl: 6, pr: 2, pb: 2 }}>

                          {/* Step definitions */}
                          <Typography variant="subtitle2" sx={{ mb: 1, fontSize: '0.8rem', color: '#5f6368' }}>
                            Steps
                          </Typography>
                          <Stack spacing={1} sx={{ mb: 2 }}>
                            {(b.steps || []).map((step, idx) => (
                              <Box
                                key={idx}
                                sx={{ bgcolor: '#202124', color: '#fff', p: 1.5, borderRadius: '4px', fontFamily: 'monospace', fontSize: '0.8rem' }}
                              >
                                <Typography variant="caption" sx={{ color: '#8ab4f8', mb: 0.5, display: 'block' }}>
                                  Step #{idx}: {step.name}
                                </Typography>
                                <Typography variant="body2" sx={{ fontSize: '0.75rem' }}>
                                  $ {step.args?.join(' ')}
                                </Typography>
                              </Box>
                            ))}
                          </Stack>

                          {/* Live log output */}
                          <Typography variant="subtitle2" sx={{ mb: 1, fontSize: '0.8rem', color: '#5f6368', display: 'flex', alignItems: 'center', gap: 1 }}>
                            Build Logs
                            {fetchingLogs && <CircularProgress size={10} />}
                          </Typography>
                          <Box
                            sx={{
                              bgcolor: '#0d1117',
                              color: '#c9d1d9',
                              borderRadius: '4px',
                              p: 1.5,
                              fontFamily: 'monospace',
                              fontSize: '0.75rem',
                              maxHeight: 280,
                              overflowY: 'auto',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                              lineHeight: 1.6,
                            }}
                          >
                            {logs.length === 0 ? (
                              <Typography sx={{ color: '#484f58', fontSize: '0.75rem', fontFamily: 'monospace' }}>
                                {fetchingLogs ? 'Loading logs...' : 'No log output yet.'}
                              </Typography>
                            ) : (
                              logs.map((entry, i) => (
                                <Box key={i} sx={{ mb: 0.25 }}>
                                  <Box component="span" sx={{ color: '#6e7681', mr: 1, userSelect: 'none' }}>
                                    {new Date(entry.timestamp).toLocaleTimeString()}
                                  </Box>
                                  <Box
                                    component="span"
                                    sx={{ color: getSeverityColor(entry.severity), mr: 1, fontWeight: 700, fontSize: '0.65rem' }}
                                  >
                                    [{entry.severity}]
                                  </Box>
                                  <Box component="span">{entry.message}</Box>
                                </Box>
                              ))
                            )}
                            <div ref={logBottomRef} />
                          </Box>

                          {/* Timing */}
                          {(b.startTime || b.finishTime) && (
                            <Box sx={{ mt: 2, pt: 2, borderTop: '1px dashed #eee' }}>
                              {b.startTime && (
                                <Typography variant="caption" sx={{ display: 'block' }} color="textSecondary">
                                  Started: {new Date(b.startTime).toLocaleString()}
                                </Typography>
                              )}
                              {b.finishTime && (
                                <Typography variant="caption" sx={{ display: 'block' }} color="textSecondary">
                                  Finished: {new Date(b.finishTime).toLocaleString()}
                                </Typography>
                              )}
                            </Box>
                          )}
                        </Box>
                      </Collapse>
                    </Box>
                  );
                })
            )}
          </List>
        )}
      </Box>
    </Drawer>
  );
}
