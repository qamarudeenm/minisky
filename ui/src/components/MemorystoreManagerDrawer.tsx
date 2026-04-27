import { useState, useEffect } from 'react';
import {
  Box, Typography, Button, Drawer, IconButton, Table, TableBody, TableCell,
  TableHead, TableRow, Chip, Alert, CircularProgress, Tooltip,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, MenuItem,
  Select, FormControl, InputLabel,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import AddIcon from '@mui/icons-material/Add';
import CheckCircleOutlinedIcon from '@mui/icons-material/CheckCircleOutlined';
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';

import { useProjectContext } from '../contexts/ProjectContext';

type Props = { open: boolean; onClose: () => void };

type Instance = {
  name: string;
  tier: string;
  memorySizeGb: number;
  host: string;
  port: number;
  state: string;
  createTime: string;
  engineVersion?: string;
};

const TIERS = ['BASIC', 'STANDARD_HA'];
const VERSIONS = [
  { id: 'REDIS_8_0', label: 'Redis 8.0 (Latest)', type: 'redis' },
  { id: 'REDIS_7_4', label: 'Redis 7.4', type: 'redis' },
  { id: 'REDIS_7_2', label: 'Redis 7.2 (LTS)', type: 'redis' },
  { id: 'VALKEY_9_0', label: 'Valkey 9.0 (Latest)', type: 'valkey' },
  { id: 'VALKEY_8_1', label: 'Valkey 8.1', type: 'valkey' },
  { id: 'VALKEY_7_2', label: 'Valkey 7.2', type: 'valkey' },
  { id: 'MEMCACHED_1_6_33', label: 'Memcached 1.6.33 (Latest)', type: 'memcache' },
  { id: 'MEMCACHED_1_6_32', label: 'Memcached 1.6.32', type: 'memcache' },
  { id: 'MEMCACHED_1_5_22', label: 'Memcached 1.5.22 (Legacy)', type: 'memcache' },
];

function StateChip({ state }: { state: string }) {
  const isReady = state === 'READY';
  const isPending = ['CREATING', 'DELETING', 'REPAIRING'].includes(state);
  
  return (
    <Chip
      size="small"
      icon={isReady ? <CheckCircleOutlinedIcon sx={{ fontSize: '14px !important' }} /> : isPending ? <HourglassEmptyIcon sx={{ fontSize: '14px !important' }} /> : undefined}
      label={state}
      sx={{
        height: 22,
        fontSize: '0.65rem',
        fontWeight: 700,
        backgroundColor: isReady ? '#e6f4ea' : isPending ? '#feefc3' : '#fce8e6',
        color: isReady ? '#1e8e3e' : isPending ? '#ea8600' : '#c5221f',
        '& .MuiChip-icon': { color: 'inherit' },
      }}
    />
  );
}

export default function MemorystoreManagerDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  
  const [form, setForm] = useState({
    id: '',
    tier: 'BASIC',
    memorySizeGb: 1,
    engineVersion: 'REDIS_8_0',
  });

  const fetchInstances = async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      // GCP Redis API usually scopes instances under locations, but for the shim we use a simple project filter
      const res = await fetch(`/api/manage/memorystore/projects/${activeProject}/locations/-/instances`);
      if (res.ok) {
        const data = await res.json();
        setInstances(data.instances || []);
      }
    } catch {
      if (!silent) setError('Failed to reach Memorystore API');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      fetchInstances();
      const t = setInterval(() => fetchInstances(true), 5000);
      return () => clearInterval(t);
    }
  }, [open, activeProject]);

  const handleCreate = async () => {
    setCreateOpen(false);
    setLoading(true);
    try {
      const isRedis = form.engineVersion.startsWith('REDIS') || form.engineVersion.startsWith('VALKEY');
      const base = isRedis ? 'redis.googleapis.com' : 'memcache.googleapis.com';
      const endpoint = `/api/manage/memorystore/projects/${activeProject}/locations/us-central1/instances`;
      
      const res = await fetch(endpoint + `?instanceId=${form.id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Host': base },
        body: JSON.stringify({
          tier: form.tier,
          memorySizeGb: form.memorySizeGb,
          engineVersion: form.engineVersion,
        }),
      });
      if (res.ok) fetchInstances();
      else setError('Create failed');
    } catch {
      setError('Network error');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (instanceName: string) => {
    const id = instanceName.split('/').pop();
    if (!confirm(`Delete instance "${id}"?`)) return;
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/memorystore/${instanceName}`, { method: 'DELETE' });
      if (res.ok) fetchInstances();
      else setError('Delete failed');
    } catch {
      setError('Network error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 700, p: 4, height: '100%', display: 'flex', flexDirection: 'column' }}>
        
        {/* Header */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Box sx={{ p: 1, borderRadius: '8px', background: '#fce8e6', color: '#c5221f' }}>
              <StorageIcon />
            </Box>
            <Box>
              <Typography variant="h5" sx={{ fontWeight: 500 }}>Memorystore</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Redis & Memcached Instances</Typography>
            </Box>
          </Box>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button variant="contained" size="small" startIcon={<AddIcon />} onClick={() => setCreateOpen(true)}>
              Create Instance
            </Button>
            <IconButton onClick={() => fetchInstances()} disabled={loading} size="small">
              <RefreshIcon fontSize="small" />
            </IconButton>
            <IconButton onClick={onClose}><CloseIcon /></IconButton>
          </Box>
        </Box>

        {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

        <Table size="small" stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Instance ID</TableCell>
              <TableCell>Engine</TableCell>
              <TableCell>Tier</TableCell>
              <TableCell>Status</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading && instances.length === 0 ? (
              <TableRow><TableCell colSpan={5} align="center" sx={{ py: 4 }}><CircularProgress size={24} /></TableCell></TableRow>
            ) : instances.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ py: 8 }}>
                  <Typography variant="body2" sx={{ color: '#5f6368' }}>No instances found</Typography>
                </TableCell>
              </TableRow>
            ) : (
              instances.map(inst => {
                const id = inst.name.split('/').pop();
                return (
                  <TableRow key={inst.name} hover>
                    <TableCell>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>{id}</Typography>
                      <Typography variant="caption" sx={{ display: 'block', color: '#80868b' }}>{inst.host}:{inst.port}</Typography>
                    </TableCell>
                    <TableCell>
                      <Chip label={inst.engineVersion?.split('_')[0] || 'REDIS'} size="small" variant="outlined" sx={{ fontSize: '0.65rem', height: 20 }} />
                    </TableCell>
                    <TableCell><Typography variant="caption">{inst.tier}</Typography></TableCell>
                    <TableCell><StateChip state={inst.state} /></TableCell>
                    <TableCell align="right">
                      <Tooltip title="Delete">
                        <IconButton size="small" color="error" onClick={() => handleDelete(inst.name)}>
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>

        <Box sx={{ mt: 'auto', p: 2, bgcolor: '#f8f9fa', borderRadius: 2, display: 'flex', gap: 1.5, alignItems: 'flex-start' }}>
          <InfoOutlinedIcon sx={{ color: '#1a73e8', fontSize: 20, mt: 0.2 }} />
          <Typography variant="caption" sx={{ color: '#3c4043' }}>
            Memorystore instances in MiniSky run as isolated Docker containers using official OSS images. 
            Standard ports are mapped to allow direct connection from your local environment.
          </Typography>
        </Box>
      </Box>

      {/* Create Dialog */}
      <Dialog open={createOpen} onClose={() => setCreateOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Create Instance</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
            <TextField
              fullWidth
              label="Instance ID"
              placeholder="e.g. my-cache"
              size="small"
              value={form.id}
              onChange={e => setForm({ ...form, id: e.target.value })}
            />
            <FormControl fullWidth size="small">
              <InputLabel>Engine Version</InputLabel>
              <Select
                value={form.engineVersion}
                label="Engine Version"
                onChange={e => setForm({ ...form, engineVersion: e.target.value })}
              >
                {VERSIONS.map(v => <MenuItem key={v.id} value={v.id}>{v.label}</MenuItem>)}
              </Select>
            </FormControl>
            <FormControl fullWidth size="small">
              <InputLabel>Tier</InputLabel>
              <Select
                value={form.tier}
                label="Tier"
                onChange={e => setForm({ ...form, tier: e.target.value })}
              >
                {TIERS.map(t => <MenuItem key={t} value={t}>{t}</MenuItem>)}
              </Select>
            </FormControl>
            <TextField
              fullWidth
              label="Capacity (GB)"
              type="number"
              size="small"
              value={form.memorySizeGb}
              onChange={e => setForm({ ...form, memorySizeGb: parseInt(e.target.value) })}
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleCreate} disabled={!form.id}>Create</Button>
        </DialogActions>
      </Dialog>
    </Drawer>
  );
}
