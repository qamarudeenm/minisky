import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  Table, TableBody, TableCell, TableHead, TableRow,
  TextField, Chip, Snackbar, Alert, Select, MenuItem,
  FormControl, InputLabel, CircularProgress, Tooltip,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import StopIcon from '@mui/icons-material/Stop';
import TerminalIcon from '@mui/icons-material/Terminal';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useProjectContext } from '../contexts/ProjectContext';
import TerminalDrawer from './TerminalDrawer';

const ZONE = 'us-central1-a';

// Dynamic OS images loaded from /api/config/images

type Instance = {
  name: string;
  id: string;
  status: string;
  machineType: string;
  description: string;
  networkInterfaces: { networkIP: string, network?: string }[];
  hostPorts?: { ContainerPort: string, HostPort: string }[];
  creationTimestamp: string;
};

type ComputeManagerDrawerProps = { open: boolean; onClose: () => void };

export default function ComputeManagerDrawer({ open, onClose }: ComputeManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(false);
  const [vmName, setVmName] = useState('');
  const [osImage, setOsImage] = useState('ubuntu:26.04');
  const [availableImages, setAvailableImages] = useState<{label: string, image: string}[]>([]);
  const [machineType, setMachineType] = useState('n1-standard-1');
  const [vpcName, setVpcName] = useState('default');
  const [networks, setNetworks] = useState<{name: string}[]>([]);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });
  const [terminalContainer, setTerminalContainer] = useState<string | null>(null);

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadInstances = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/manage/compute/projects/${activeProject}/zones/${ZONE}/instances`
      );
      if (res.ok) {
        const data = await res.json();
        setInstances(data.items || []);
      }
    } catch (e) {
      console.error(e);
    }
  }, [activeProject]);

  useEffect(() => {
    if (open) {
      loadInstances();
      
      fetch(`/api/manage/compute/projects/${activeProject}/global/networks`)
        .then(r => r.json())
        .then(d => setNetworks(d.items || []))
        .catch(console.error);

      fetch('/api/config/images')
        .then(r => r.json())
        .then(d => {
          if (d.compute?.os_images) {
            setAvailableImages(d.compute.os_images);
          }
        })
        .catch(console.error);

      // Poll status every 1s to catch PROVISIONING→RUNNING transitions
      const t = setInterval(loadInstances, 1000);
      return () => clearInterval(t);
    }
  }, [open, loadInstances, activeProject]);

  const handleCreate = async () => {
    if (!vmName.trim()) return;
    setLoading(true);
    try {
      const res = await fetch(
        `/api/manage/compute/projects/${activeProject}/zones/${ZONE}/instances`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: vmName.trim(),
            machineType,
            networkInterfaces: [{ network: `https://www.googleapis.com/compute/v1/projects/${activeProject}/global/networks/${vpcName}` }],
            disks: [{ source: osImage, boot: true, autoDelete: true }],
          }),
        }
      );
      if (res.ok) {
        showToast(`VM "${vmName}" is being provisioned — Docker image: ${osImage}`);
        setVmName('');
        setTimeout(loadInstances, 1500);
      } else {
        const e = await res.json();
        showToast(e?.error?.message || 'Failed to create VM', 'error');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (name: string) => {
    await fetch(
      `/api/manage/compute/projects/${activeProject}/zones/${ZONE}/instances/${name}`,
      { method: 'DELETE' }
    );
    showToast(`VM "${name}" deletion requested`);
    setTimeout(loadInstances, 1000);
  };

  const handleAction = async (name: string, action: 'start' | 'stop') => {
    await fetch(
      `/api/manage/compute/projects/${activeProject}/zones/${ZONE}/instances/${name}/${action}`,
      { method: 'POST' }
    );
    showToast(`VM "${name}" ${action} requested`);
    setTimeout(loadInstances, 1000);
  };

  const sshCommand = (name: string) => `docker exec -it minisky-vm-${name} /bin/bash`;

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    showToast('SSH command copied to clipboard!');
  };

  const statusColor = (status: string): 'success' | 'warning' | 'error' | 'default' => {
    switch (status) {
      case 'RUNNING': return 'success';
      case 'STAGING':
      case 'PROVISIONING': return 'warning';
      case 'DELETING':
      case 'TERMINATED': return 'error';
      default: return 'default';
    }
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ width: 760, p: 4 }}>
          {/* Header */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
            <Typography variant="h5" sx={{ fontWeight: 500 }}>Compute Engine</Typography>
            <IconButton onClick={onClose}><CloseIcon /></IconButton>
          </Box>
          <Typography variant="body2" sx={{ color: '#5f6368', mb: 3 }}>
            Project:{' '}
            <Chip label={activeProject} size="small" sx={{ ml: 1, backgroundColor: '#e8f0fe', color: '#1a73e8', fontWeight: 600 }} />
            {' '}Zone:{' '}
            <Chip label={ZONE} size="small" variant="outlined" sx={{ ml: 1 }} />
          </Typography>

          {/* Launch VM form */}
          <Box sx={{ display: 'flex', gap: 1.5, mb: 4, flexWrap: 'wrap', alignItems: 'flex-end' }}>
            <TextField
              size="small"
              label="VM Name"
              placeholder="my-vm"
              value={vmName}
              onChange={e => setVmName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
              sx={{ flex: 1, minWidth: 150 }}
            />
            <FormControl size="small" sx={{ minWidth: 140 }}>
              <InputLabel>Machine Type</InputLabel>
              <Select value={machineType} label="Machine Type" onChange={e => setMachineType(e.target.value)}>
                <MenuItem value="n1-standard-1">n1-standard-1</MenuItem>
                <MenuItem value="n1-standard-2">n1-standard-2</MenuItem>
                <MenuItem value="e2-micro">e2-micro</MenuItem>
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel>OS Image</InputLabel>
              <Select value={osImage} label="OS Image" onChange={e => setOsImage(e.target.value)}>
                {availableImages.map((img) => (
                  <MenuItem key={img.image} value={img.image}>{img.label}</MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 140 }}>
              <InputLabel>VPC Network</InputLabel>
              <Select value={vpcName} label="VPC Network" onChange={e => setVpcName(e.target.value)}>
                <MenuItem value="default">default</MenuItem>
                {networks.filter(n => n.name !== 'default').map(net => (
                  <MenuItem key={net.name} value={net.name}>{net.name}</MenuItem>
                ))}
              </Select>
            </FormControl>
            <Button
              variant="contained"
              onClick={handleCreate}
              disabled={loading || !vmName.trim()}
              startIcon={loading ? <CircularProgress size={16} color="inherit" /> : <PlayArrowIcon />}
              sx={{ whiteSpace: 'nowrap', height: 40 }}
            >
              Launch VM
            </Button>
          </Box>

          {/* Instance table */}
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Machine Type</TableCell>
                <TableCell>Internal IP</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {instances.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} align="center" sx={{ py: 4, color: '#80868b' }}>
                    No VM instances found. Launch one above!
                  </TableCell>
                </TableRow>
              )}
              {instances.map(inst => {
                const shortType = inst.machineType.split('/').pop() || inst.machineType;
                const ip = inst.networkInterfaces?.[0]?.networkIP || '—';
                return (
                  <TableRow key={inst.name} hover>
                    <TableCell>
                      <Typography variant="body2" sx={{ fontWeight: 600 }}>{inst.name}</Typography>
                      <Typography variant="caption" sx={{ color: '#80868b', fontFamily: 'monospace' }}>
                        {inst.description || `minisky-vm-${inst.name}`}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={inst.status}
                        size="small"
                        color={statusColor(inst.status)}
                        variant={inst.status === 'RUNNING' ? 'filled' : 'outlined'}
                      />
                    </TableCell>
                    <TableCell>
                      <Chip label={shortType} size="small" variant="outlined" />
                    </TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                      <Box>{ip}</Box>
                      {inst.hostPorts && inst.hostPorts.length > 0 && (
                        <Box sx={{ mt: 0.5 }}>
                          {inst.hostPorts.map(p => (
                            <Chip key={p.ContainerPort} size="small" variant="outlined" color="primary" 
                                  label={`${p.ContainerPort} → :${p.HostPort}`} sx={{ fontSize: '0.7rem', height: 20, mr: 0.5 }} />
                          ))}
                        </Box>
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Box sx={{ display: 'flex', gap: 0.5, justifyContent: 'flex-end' }}>
                        <Tooltip title={`SSH into ${inst.name}`}>
                          <IconButton
                            size="small"
                            sx={{ color: '#1a73e8' }}
                            onClick={() => setTerminalContainer(`minisky-vm-${inst.name}`)}
                          >
                            <TerminalIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Copy SSH command (bash)">
                          <IconButton size="small" onClick={() => copyToClipboard(sshCommand(inst.name))}>
                            <ContentCopyIcon sx={{ fontSize: 14 }} />
                          </IconButton>
                        </Tooltip>
                        {inst.status === 'TERMINATED' ? (
                          <Tooltip title="Start instance">
                            <IconButton size="small" color="success" onClick={() => handleAction(inst.name, 'start')}>
                              <PlayArrowIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        ) : (
                          <Tooltip title="Stop instance">
                            <IconButton size="small" color="warning" onClick={() => handleAction(inst.name, 'stop')}>
                              <StopIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        )}
                        <Tooltip title="Delete instance">
                          <IconButton size="small" color="error" onClick={() => handleDelete(inst.name)}>
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Box>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>

          {/* SSH hint */}
          {instances.some(i => i.status === 'RUNNING') && (
            <Box sx={{ mt: 3, p: 2, bgcolor: '#f8f9fa', borderRadius: 2, border: '1px solid #dadce0' }}>
              <Typography variant="caption" sx={{ fontWeight: 600, display: 'block', mb: 1 }}>
                💡 SSH into any running VM
              </Typography>
              <Typography variant="caption" sx={{ fontFamily: 'monospace', color: '#5f6368' }}>
                docker exec -it minisky-vm-&lt;name&gt; /bin/bash
              </Typography>
            </Box>
          )}
        </Box>
      </Drawer>

      <TerminalDrawer 
        open={!!terminalContainer} 
        onClose={() => setTerminalContainer(null)} 
        containerName={terminalContainer || ''} 
      />

      <Snackbar
        open={toast.open}
        autoHideDuration={1500}
        onClose={() => setToast(t => ({ ...t, open: false }))}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert severity={toast.severity} sx={{ width: '100%' }}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
