import { useState, useEffect } from 'react';
import { 
  Drawer, Box, Typography, TextField, Button, 
  Stack, Divider, Alert, List, ListItem, ListItemText, IconButton,
  CircularProgress, Collapse, Paper, Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
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

  const fetchBuilds = async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/cloudbuild/v1/projects/${activeProject}/builds`);
      if (!res.ok) throw new Error('Failed to fetch builds');
      const data = await res.json();
      setBuilds(data.builds || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      fetchBuilds();
    }
  }, [open, activeProject]);

  const handleSubmitBuild = async () => {
    setSubmitting(true);
    setError(null);
    try {
      let body;
      try {
        body = JSON.parse(buildConfig);
      } catch (e) {
        throw new Error('Invalid JSON configuration');
      }

      const payload = {
        ...body,
        source: sourceRepo ? {
          repoSource: {
            repoName: sourceRepo,
            branchName: sourceBranch
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

      fetchBuilds();
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

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 600, p: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h5" sx={{ fontWeight: 500 }}>Cloud Build</Typography>
          <IconButton onClick={onClose}><CloseIcon /></IconButton>
        </Box>

        <Divider sx={{ mb: 4 }} />

        {error && <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>{error}</Alert>}

        <Paper variant="outlined" sx={{ mb: 6, p: 3, background: '#f8f9fa' }}>
          <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Submit New Build</Typography>
          <Box sx={{ mb: 3 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 1, color: '#5f6368', fontWeight: 600 }}>SOURCE REPOSITORY (OPTIONAL)</Typography>
            <Box sx={{ display: 'flex', gap: 2 }}>
              <TextField 
                fullWidth 
                size="small" 
                placeholder="Repository (e.g. github.com/user/repo)" 
                value={sourceRepo}
                onChange={(e) => setSourceRepo(e.target.value)}
                sx={{ backgroundColor: '#fff' }}
              />
              <TextField 
                sx={{ width: 120, backgroundColor: '#fff' }} 
                size="small" 
                placeholder="Branch" 
                value={sourceBranch}
                onChange={(e) => setSourceBranch(e.target.value)}
              />
            </Box>
          </Box>

          <Typography variant="caption" sx={{ display: 'block', mb: 1, color: '#5f6368', fontWeight: 600 }}>BUILD CONFIGURATION (JSON)</Typography>
          <TextField 
            multiline 
            rows={6} 
            fullWidth 
            variant="outlined" 
            value={buildConfig}
            onChange={(e) => setBuildConfig(e.target.value)}
            placeholder='{"steps": [{"name": "ubuntu", "args": ["echo", "hello"]}]}'
            sx={{ mb: 2, '& .MuiInputBase-root': { fontFamily: 'monospace', fontSize: '0.85rem' } }}
          />
          <Button 
            variant="contained" 
            startIcon={<PlayArrowIcon />}
            onClick={handleSubmitBuild}
            disabled={submitting || !buildConfig}
            fullWidth
          >
            {submitting ? 'Submitting...' : 'Submit Build'}
          </Button>
        </Paper>

        <Typography variant="h6" sx={{ mb: 2, fontSize: '1.1rem' }}>Build History in {activeProject}</Typography>
        
        {loading ? (
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
              builds.sort((a, b) => b.createTime.localeCompare(a.createTime)).map((b) => {
                const isExpanded = expandedBuild === b.id;
                return (
                  <Box key={b.id} sx={{ mb: 1, borderBottom: '1px solid #eee' }}>
                    <ListItem sx={{ px: 1 }}>
                      <IconButton size="small" onClick={() => setExpandedBuild(isExpanded ? null : b.id)} sx={{ mr: 1 }}>
                        {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                      </IconButton>
                      <ListItemText 
                        primary={<Typography sx={{ fontWeight: 600, fontSize: '0.9rem' }}>{b.id}</Typography>}
                        secondary={
                          <Box>
                            <Typography variant="caption" sx={{ display: 'block' }}>Created: {new Date(b.createTime).toLocaleString()}</Typography>
                            {b.source && (
                              <Typography variant="caption" sx={{ color: '#1a73e8', fontWeight: 600 }}>
                                Source: {b.source.repoSource.repoName} ({b.source.repoSource.branchName})
                              </Typography>
                            )}
                          </Box>
                        }
                      />
                      <Chip 
                        size="small" 
                        label={b.status} 
                        color={getStatusColor(b.status) as any} 
                        variant="filled"
                        sx={{ fontWeight: 600, fontSize: '0.7rem' }}
                      />
                    </ListItem>
                    
                    <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                      <Box sx={{ pl: 6, pr: 2, pb: 2 }}>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontSize: '0.8rem', color: '#5f6368' }}>Steps</Typography>
                        <Stack spacing={1}>
                          {b.steps.map((step, idx) => (
                            <Box key={idx} sx={{ bgcolor: '#202124', color: '#fff', p: 1.5, borderRadius: '4px', fontFamily: 'monospace', fontSize: '0.8rem' }}>
                              <Typography variant="caption" sx={{ color: '#8ab4f8', mb: 0.5, display: 'block' }}>Step #{idx}: {step.name}</Typography>
                              <Typography variant="body2" sx={{ fontSize: '0.75rem' }}>$ {step.args?.join(' ')}</Typography>
                            </Box>
                          ))}
                        </Stack>
                        
                        {(b.startTime || b.finishTime) && (
                          <Box sx={{ mt: 2, pt: 2, borderTop: '1px dashed #eee' }}>
                            {b.startTime && <Typography variant="caption" sx={{ display: 'block' }} color="textSecondary">Started: {new Date(b.startTime).toLocaleString()}</Typography>}
                            {b.finishTime && <Typography variant="caption" sx={{ display: 'block' }} color="textSecondary">Finished: {new Date(b.finishTime).toLocaleString()}</Typography>}
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
