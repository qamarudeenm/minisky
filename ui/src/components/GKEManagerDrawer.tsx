import { useState, useEffect } from 'react';
import { Box, Typography, Button, Drawer, IconButton, Table, TableBody, TableCell, TableHead, TableRow, Chip, Alert, CircularProgress, Tooltip, TextField } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import HubIcon from '@mui/icons-material/Hub';
import StorageIcon from '@mui/icons-material/Storage';
import DownloadIcon from '@mui/icons-material/Download';

type Props = {
  open: boolean;
  onClose: () => void;
};

type GkeCluster = {
  name: string;
  status: 'RUNNING' | 'PROVISIONING';
};

export default function GKEManagerDrawer({ open, onClose }: Props) {
  const [clusters, setClusters] = useState<GkeCluster[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [newClusterName, setNewClusterName] = useState('');
  const [provisioning, setProvisioning] = useState(false);

  const fetchClusters = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/manage/gke/clusters');
      if (res.ok) {
        const data = await res.json();
        setClusters(data || []);
      } else {
        const text = await res.text();
        setError(text);
      }
    } catch (err) {
      setError('Failed to connect to backend API');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!newClusterName) return;
    setProvisioning(true);
    try {
      const res = await fetch('/api/manage/gke/clusters', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newClusterName })
      });
      if (res.ok) {
        setNewClusterName('');
        // Immediately fetch to show the provisioning state
        fetchClusters();
      } else {
        const text = await res.text();
        setError(text);
      }
    } catch (err) {
      setError('Failed to provision cluster');
    } finally {
      setProvisioning(false);
    }
  };

  const handleDownloadConfig = async (name: string) => {
    const res = await fetch(`/api/manage/gke/clusters/${name}/config`);
    if (res.ok) {
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${name}-kubeconfig.yaml`;
      a.click();
    } else {
      alert('Failed to download kubeconfig');
    }
  };

  const handleDelete = async (name: string) => {
    if (!confirm(`Are you sure you want to delete GKE cluster "${name}"? This action is IRREVERSIBLE.`)) return;
    
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/gke/clusters/${name}`, { method: 'DELETE' });
      if (res.ok) {
        fetchClusters();
      } else {
        const text = await res.text();
        alert(`Delete failed: ${text}`);
      }
    } catch (err) {
      alert('Network error during deletion');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      fetchClusters();
      
      // Setup polling every 5 seconds while drawer is open
      const timer = setInterval(() => {
        fetchClusters(true);
      }, 5000);
      
      return () => clearInterval(timer);
    }
  }, [open]);

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: '700px', p: 4, height: '100%', display: 'flex', flexDirection: 'column' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Box sx={{ p: 1, borderRadius: '8px', background: '#e8f0fe', color: '#1a73e8' }}>
              <HubIcon />
            </Box>
            <Typography variant="h5" sx={{ fontWeight: 500 }}>GKE Cluster Console</Typography>
          </Box>
          <Box>
            <IconButton onClick={() => fetchClusters()} disabled={loading} size="small" sx={{ mr: 1 }}>
              <RefreshIcon fontSize="small" />
            </IconButton>
            <IconButton onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </Box>
        </Box>

        <Typography variant="body2" sx={{ color: '#5f6368', mb: 3 }}>
          Managing local <strong>kind</strong> (Kubernetes-in-Docker) clusters. These environments simulate Google Kubernetes Engine (GKE).
        </Typography>

        <Box sx={{ mb: 4, display: 'flex', gap: 2 }}>
          <TextField 
            size="small" 
            label="New Cluster Name" 
            placeholder="e.g. staging-cluster" 
            value={newClusterName} 
            onChange={(e) => setNewClusterName(e.target.value)} 
            fullWidth 
            disabled={provisioning}
          />
          <Button 
            variant="contained" 
            onClick={handleCreate} 
            disabled={provisioning || !newClusterName}
            sx={{ whiteSpace: 'nowrap', minWidth: '160px' }}
          >
            {provisioning ? <CircularProgress size={20} color="inherit" /> : 'Provision Cluster'}
          </Button>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>{error}</Alert>
        )}

        <Box sx={{ flexGrow: 1, overflow: 'auto' }}>
          <Table size="small" stickyHeader>
            <TableHead>
              <TableRow>
                <TableCell>Cluster Architecture</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Endpoint</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {loading && clusters.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 6 }}>
                    <CircularProgress size={24} />
                    <Typography variant="body2" sx={{ mt: 2, color: '#80868b' }}>Querying orchestration layer...</Typography>
                  </TableCell>
                </TableRow>
              ) : clusters.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 8 }}>
                    <StorageIcon sx={{ fontSize: 48, color: '#dadce0', mb: 2 }} />
                    <Typography variant="body1" sx={{ color: '#5f6368' }}>No active GKE clusters detected</Typography>
                    <Typography variant="caption" sx={{ color: '#80868b' }}>Provision a new cluster above to begin</Typography>
                  </TableCell>
                </TableRow>
              ) : (
                clusters.map((c) => (
                  <TableRow key={c.name} hover sx={{ opacity: c.status === 'PROVISIONING' ? 0.7 : 1 }}>
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                        <HubIcon sx={{ color: c.status === 'PROVISIONING' ? '#5f6368' : '#1a73e8', fontSize: 18 }} />
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>{c.name}</Typography>
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Chip 
                        label={c.status} 
                        size="small" 
                        sx={{ 
                          height: '20px', 
                          fontSize: '0.65rem', 
                          fontWeight: 700, 
                          backgroundColor: c.status === 'PROVISIONING' ? '#feefc3' : '#e6f4ea', 
                          color: c.status === 'PROVISIONING' ? '#ea8600' : '#1e8e3e',
                          animation: c.status === 'PROVISIONING' ? 'pulse 1.5s infinite' : 'none'
                        }} 
                      />
                    </TableCell>
                    <TableCell>
                      <Typography variant="caption" sx={{ fontFamily: 'monospace', color: c.status === 'PROVISIONING' ? '#80868b' : '#1a73e8' }}>
                        {c.status === 'PROVISIONING' ? 'Assigning IP...' : '127.0.0.1 (Local)'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title="Get Credentials (Kubeconfig)">
                        <IconButton size="small" color="primary" onClick={() => handleDownloadConfig(c.name)} disabled={c.status === 'PROVISIONING'} sx={{ mr: 1 }}>
                          <DownloadIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title={c.status === 'PROVISIONING' ? 'Provisioning in progress...' : 'Deprovision Cluster'}>
                        <span>
                          <IconButton size="small" color="error" onClick={() => handleDelete(c.name)} disabled={c.status === 'PROVISIONING'}>
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </Box>

        <Box sx={{ mt: 3, pt: 3, borderTop: '1px solid #dadce0', display: 'flex', gap: 2 }}>
          <Box sx={{ background: '#f8f9fa', p: 2, borderRadius: '8px', flex: 1 }}>
            <Typography variant="caption" sx={{ fontWeight: 600, color: '#3c4043', display: 'block', mb: 1 }}>
              Quick Connect & Deploy
            </Typography>
            <Box sx={{ 
              backgroundColor: '#202124', 
              color: '#f8f9fa', 
              p: 1.5, 
              borderRadius: '4px',
              fontFamily: 'monospace',
              fontSize: '0.75rem',
              overflowX: 'auto',
              mb: 1
            }}>
              export KUBECONFIG=$PWD/[NAME]-kubeconfig.yaml<br/>
              kubectl cluster-info
            </Box>
            <Typography variant="caption" sx={{ color: '#5f6368', display: 'block' }}>
              First-time provisioning pulls a large Docker image (~300MB) which can take 5-10 minutes. Subsequent clusters load in seconds.
            </Typography>
          </Box>
        </Box>

        <style>{`
          @keyframes pulse {
            0% { opacity: 1; }
            50% { opacity: 0.5; }
            100% { opacity: 1; }
          }
        `}</style>
      </Box>
    </Drawer>
  );
}
