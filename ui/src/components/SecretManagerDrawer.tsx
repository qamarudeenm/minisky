import { useState, useEffect } from 'react';
import { 
  Drawer, Box, Typography, TextField, Button, 
  Stack, Divider, Alert, List, ListItem, ListItemText, ListItemSecondaryAction, IconButton,
  CircularProgress, Tooltip, Collapse, Paper
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import VisibilityIcon from '@mui/icons-material/Visibility';
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff';
import { useProjectContext } from '../contexts/ProjectContext';

interface Secret {
  name: string;
  createTime: string;
}

interface SecretVersion {
  name: string;
  createTime: string;
  state: string;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function SecretManagerDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [secrets, setSecrets] = useState<Secret[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const [newSecretId, setNewSecretId] = useState('');
  const [initialValue, setInitialValue] = useState('');
  const [creating, setCreating] = useState(false);

  const [expandedSecret, setExpandedSecret] = useState<string | null>(null);
  const [versions, setVersions] = useState<SecretVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [newVersionValue, setNewVersionValue] = useState('');
  const [addingVersion, setAddingVersion] = useState(false);
  const [revealedVersion, setRevealedVersion] = useState<string | null>(null);
  const [versionValue, setVersionValue] = useState<string | null>(null);

  const fetchSecrets = async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/secretmanager/projects/${activeProject}/secrets`);
      if (!res.ok) throw new Error('Failed to fetch secrets');
      const data = await res.json();
      setSecrets(data.secrets || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchVersions = async (secretName: string) => {
    setLoadingVersions(true);
    setRevealedVersion(null);
    setVersionValue(null);
    try {
      const res = await fetch(`/api/manage/secretmanager/${secretName}/versions`);
      if (!res.ok) throw new Error('Failed to fetch versions');
      const data = await res.json();
      setVersions(data.versions || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoadingVersions(false);
    }
  };

  useEffect(() => {
    if (open) {
      fetchSecrets();
    } else {
      setExpandedSecret(null);
      setVersions([]);
    }
  }, [open, activeProject]);

  const handleCreate = async () => {
    setCreating(true);
    setError(null);
    try {
      const res = await fetch(`/api/manage/secretmanager/projects/${activeProject}/secrets?secretId=${newSecretId}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ replication: { automatic: {} } })
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error?.message || 'Failed to create secret');
      }

      if (initialValue) {
        await fetch(`/api/manage/secretmanager/projects/${activeProject}/secrets/${newSecretId}:addVersion`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ payload: { data: btoa(initialValue) } })
        });
      }

      setNewSecretId('');
      setInitialValue('');
      fetchSecrets();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleAddVersion = async (secretName: string) => {
    const secretId = secretName.split('/').pop();
    setAddingVersion(true);
    try {
      const res = await fetch(`/api/manage/secretmanager/projects/${activeProject}/secrets/${secretId}:addVersion`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ payload: { data: btoa(newVersionValue) } })
      });
      if (!res.ok) throw new Error('Failed to add version');
      setNewVersionValue('');
      fetchVersions(secretName);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setAddingVersion(false);
    }
  };

  const handleAccessVersion = async (versionName: string) => {
    if (revealedVersion === versionName) {
      setRevealedVersion(null);
      setVersionValue(null);
      return;
    }

    try {
      const res = await fetch(`/api/manage/secretmanager/${versionName}:access`);
      if (!res.ok) throw new Error('Failed to access secret version');
      const data = await res.json();
      setVersionValue(atob(data.payload.data));
      setRevealedVersion(versionName);
    } catch (err: any) {
      setError(err.message);
    }
  };

  const handleDelete = async (name: string) => {
    const secretId = name.split('/').pop();
    if (!confirm(`Are you sure you want to delete secret ${secretId}?`)) return;
    
    try {
      const res = await fetch(`/api/manage/secretmanager/${name}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete secret');
      if (expandedSecret === name) setExpandedSecret(null);
      fetchSecrets();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const toggleExpand = (name: string) => {
    if (expandedSecret === name) {
      setExpandedSecret(null);
      setVersions([]);
    } else {
      setExpandedSecret(name);
      fetchVersions(name);
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 550, p: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h5" sx={{ fontWeight: 500 }}>Secret Manager</Typography>
          <IconButton onClick={onClose}><CloseIcon /></IconButton>
        </Box>

        <Divider sx={{ mb: 4 }} />

        {error && <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>{error}</Alert>}

        <Paper variant="outlined" sx={{ mb: 6, p: 3, background: '#f8f9fa' }}>
          <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Create New Secret</Typography>
          <Stack spacing={2}>
            <TextField 
              label="Secret ID" 
              size="small" 
              fullWidth 
              value={newSecretId}
              onChange={(e) => setNewSecretId(e.target.value)}
              placeholder="e.g. API_KEY"
            />
            <TextField 
              label="Secret Value (Initial Version)" 
              size="small" 
              fullWidth 
              multiline
              rows={2}
              value={initialValue}
              onChange={(e) => setInitialValue(e.target.value)}
              placeholder="Enter sensitive data here..."
            />
            <Button 
              variant="contained" 
              startIcon={<AddIcon />}
              onClick={handleCreate}
              disabled={creating || !newSecretId}
            >
              {creating ? 'Creating...' : 'Create Secret'}
            </Button>
          </Stack>
        </Paper>

        <Typography variant="h6" sx={{ mb: 2, fontSize: '1.1rem' }}>Secrets in {activeProject}</Typography>
        
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : (
          <List disablePadding>
            {secrets.length === 0 ? (
              <Typography variant="body2" sx={{ color: '#70757a', textAlign: 'center', py: 4 }}>
                No secrets found in this project.
              </Typography>
            ) : (
              secrets.map((s) => {
                const id = s.name.split('/').pop();
                const isExpanded = expandedSecret === s.name;
                return (
                  <Box key={s.name} sx={{ mb: 1, borderBottom: '1px solid #eee' }}>
                    <ListItem sx={{ px: 1 }}>
                      <IconButton size="small" onClick={() => toggleExpand(s.name)} sx={{ mr: 1 }}>
                        {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                      </IconButton>
                      <ListItemText 
                        primary={<Typography sx={{ fontWeight: 500 }}>{id}</Typography>}
                        secondary={`Created: ${new Date(s.createTime).toLocaleString()}`}
                      />
                      <ListItemSecondaryAction>
                        <Tooltip title="Copy Resource Name">
                          <IconButton size="small" onClick={() => copyToClipboard(s.name)}>
                            <ContentCopyIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <IconButton size="small" color="error" onClick={() => handleDelete(s.name)}>
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </ListItemSecondaryAction>
                    </ListItem>
                    
                    <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                      <Box sx={{ pl: 6, pr: 2, pb: 2 }}>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontSize: '0.8rem', color: '#5f6368' }}>Versions</Typography>
                        {loadingVersions ? (
                          <CircularProgress size={16} sx={{ my: 1 }} />
                        ) : (
                          <Stack spacing={1}>
                            {versions.map((v) => {
                              const vId = v.name.split('/').pop();
                              const isRevealed = revealedVersion === v.name;
                              return (
                                <Box key={v.name} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', bgcolor: '#f1f3f4', p: 1, borderRadius: '4px' }}>
                                  <Box>
                                    <Typography variant="body2" sx={{ fontWeight: 600 }}>v{vId}</Typography>
                                    <Typography variant="caption" color="textSecondary">{new Date(v.createTime).toLocaleTimeString()}</Typography>
                                  </Box>
                                  <Box sx={{ display: 'flex', alignItems: 'center' }}>
                                    {isRevealed && (
                                      <Typography variant="body2" sx={{ mr: 2, fontFamily: 'monospace', bgcolor: '#fff', px: 1, borderRadius: '2px', border: '1px solid #ddd' }}>
                                        {versionValue}
                                      </Typography>
                                    )}
                                    <Tooltip title={isRevealed ? "Hide Value" : "Reveal Value"}>
                                      <IconButton size="small" onClick={() => handleAccessVersion(v.name)}>
                                        {isRevealed ? <VisibilityOffIcon fontSize="inherit" /> : <VisibilityIcon fontSize="inherit" />}
                                      </IconButton>
                                    </Tooltip>
                                  </Box>
                                </Box>
                              );
                            })}
                            
                            <Box sx={{ mt: 2, display: 'flex', gap: 1 }}>
                              <TextField 
                                size="small" 
                                placeholder="New version value..." 
                                fullWidth
                                value={newVersionValue}
                                onChange={(e) => setNewVersionValue(e.target.value)}
                                sx={{ bgcolor: 'white' }}
                              />
                              <Button 
                                variant="outlined" 
                                size="small" 
                                onClick={() => handleAddVersion(s.name)}
                                disabled={addingVersion || !newVersionValue}
                              >
                                Add
                              </Button>
                            </Box>
                          </Stack>
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
