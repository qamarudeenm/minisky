import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText,
  Snackbar, Alert, Dialog,
  DialogTitle, DialogContent, DialogActions, Paper,
  TextField, Chip, Stack
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import FolderIcon from '@mui/icons-material/Folder';
import InventoryIcon from '@mui/icons-material/Inventory';
import HistoryIcon from '@mui/icons-material/History';
import { useProjectContext } from '../contexts/ProjectContext';

type ArtifactRegistryDrawerProps = { open: boolean; onClose: () => void };

export default function ArtifactRegistryDrawer({ open, onClose }: ArtifactRegistryDrawerProps) {
  const { activeProject } = useProjectContext();
  const [repositories, setRepositories] = useState<any[]>([]);
  const [activeRepo, setActiveRepo] = useState<any | null>(null);
  const [packages, setPackages] = useState<any[]>([]);
  const [versions, setVersions] = useState<any[]>([]);

  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });
  const [newRepoOpen, setNewRepoOpen] = useState(false);
  const [newRepoName, setNewRepoName] = useState('');

  const apiRoot = `/api/manage/artifactregistry/v1/projects/${activeProject}/locations/us-central1`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadRepositories = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/repositories`);
      if (res.ok) {
        const data = await res.json();
        setRepositories(data.repositories || []);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  const loadPackages = async (repoName: string) => {
    try {
      const res = await fetch(`/api/proxy/artifactregistry/v1/${repoName}/packages`);
      if (res.ok) {
        const data = await res.json();
        setPackages(data.packages || []);
      }
    } catch (e) { console.error(e); }
  };

  const loadVersions = async (packageName: string) => {
    try {
      const res = await fetch(`/api/proxy/artifactregistry/v1/${packageName}/versions`);
      if (res.ok) {
        const data = await res.json();
        setVersions(data.versions || []);
      }
    } catch (e) { console.error(e); }
  };

  useEffect(() => {
    if (open) {
      loadRepositories();
    } else {
      setActiveRepo(null);
      setPackages([]);
      setVersions([]);
    }
  }, [open, loadRepositories]);

  const handleCreateRepo = async () => {
    if (!newRepoName) return;
    try {
      const res = await fetch(`${apiRoot}/repositories?repositoryId=${newRepoName}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ format: 'DOCKER' })
      });
      if (res.ok) {
        showToast('Repository created successfully');
        loadRepositories();
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewRepoOpen(false);
    setNewRepoName('');
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '85vw', maxWidth: 1200, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>Artifact Registry</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Manage Docker images and language packages • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadRepositories()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            {/* Repositories Sidebar */}
            <Box sx={{ width: 300, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 2, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Repositories</Typography>
                <Button size="small" startIcon={<AddIcon />} onClick={() => setNewRepoOpen(true)}>Create</Button>
              </Box>
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {repositories.map(repo => (
                  <ListItemButton 
                    key={repo.name} 
                    selected={activeRepo?.name === repo.name}
                    onClick={() => { setActiveRepo(repo); loadPackages(repo.name); }}
                    sx={{ borderBottom: '1px solid #f1f3f4', py: 2 }}
                  >
                    <Box sx={{ mr: 2, color: '#5f6368' }}><FolderIcon /></Box>
                    <ListItemText 
                      primary={<Typography sx={{ fontSize: '0.9rem', fontWeight: activeRepo?.name === repo.name ? 600 : 400 }}>{repo.name.split('/').pop()}</Typography>} 
                      secondary={<Chip label={repo.format} size="small" sx={{ height: 16, fontSize: '0.65rem' }} />}
                    />
                  </ListItemButton>
                ))}
              </List>
            </Box>

            {/* Content Area */}
            <Box sx={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
              {activeRepo ? (
                <>
                  {/* Packages List */}
                  <Box sx={{ width: 350, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
                    <Box sx={{ p: 2, borderBottom: '1px solid #dadce0' }}>
                      <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Packages</Typography>
                    </Box>
                    <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                      {packages.map(pkg => (
                        <ListItemButton key={pkg.name} onClick={() => loadVersions(pkg.name)} sx={{ borderBottom: '1px solid #f1f3f4' }}>
                          <Box sx={{ mr: 2, color: '#1a73e8' }}><InventoryIcon /></Box>
                          <ListItemText primary={pkg.displayName || pkg.name.split('/').pop()} />
                        </ListItemButton>
                      ))}
                    </List>
                  </Box>

                  {/* Versions / Tags */}
                  <Box sx={{ flex: 1, p: 3, overflow: 'auto' }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 2 }}>Versions & Tags</Typography>
                    <Stack spacing={2}>
                      {versions.map(v => (
                        <Paper key={v.name} variant="outlined" sx={{ p: 2 }}>
                          <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                            <Typography variant="body2" sx={{ fontWeight: 600, fontFamily: 'monospace' }}>{v.name.split('/').pop()}</Typography>
                            <Box>
                              {v.relatedTags?.map((tag: string) => (
                                <Chip key={tag} label={tag} size="small" color="primary" sx={{ ml: 1 }} />
                              ))}
                            </Box>
                          </Box>
                          <Typography variant="caption" sx={{ color: '#5f6368' }}>Created: {new Date(v.createTime).toLocaleString()}</Typography>
                        </Paper>
                      ))}
                      {versions.length === 0 && (
                        <Box sx={{ textAlign: 'center', py: 10, color: '#70757a' }}>
                          <HistoryIcon sx={{ fontSize: 48, mb: 2, opacity: 0.3 }} />
                          <Typography variant="body2">Select a package to view its versions.</Typography>
                        </Box>
                      )}
                    </Stack>
                  </Box>
                </>
              ) : (
                <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#70757a' }}>
                  <Typography variant="body1">Select a repository to explore its contents.</Typography>
                </Box>
              )}
            </Box>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newRepoOpen} onClose={() => setNewRepoOpen(false)}>
        <DialogTitle>Create Repository</DialogTitle>
        <DialogContent sx={{ width: 400 }}>
          <TextField 
            autoFocus 
            margin="dense" 
            label="Repository Name" 
            fullWidth 
            variant="outlined"
            value={newRepoName} 
            onChange={e => setNewRepoName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))} 
            sx={{ mt: 1 }}
          />
          <Typography variant="caption" sx={{ mt: 1, display: 'block', color: '#5f6368' }}>
            Only lowercase letters, numbers, and hyphens are allowed.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 0 }}>
          <Button onClick={() => setNewRepoOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleCreateRepo} disabled={!newRepoName}>Create Repository</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
