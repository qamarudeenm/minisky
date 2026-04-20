import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { 
  Box, Typography, Button, Drawer, IconButton, Table, TableBody, TableCell, 
  TableHead, TableRow, Chip, Alert, CircularProgress, Tooltip, Tabs, Tab,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, MenuItem, Select,
  FormControl, InputLabel
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import CloudQueueIcon from '@mui/icons-material/CloudQueue';
import FunctionsIcon from '@mui/icons-material/Functions';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import TerminalIcon from '@mui/icons-material/Terminal';
import BuildIcon from '@mui/icons-material/Build';
import MonitorHeartIcon from '@mui/icons-material/MonitorHeart';

type Props = {
  open: boolean;
  onClose: () => void;
  isBuildpacksEnabled: boolean;
  onEnableBuildpacks: () => void;
  missingPack: boolean;
  onInstallPack: () => void;
};

type SlsResource = {
  name: string;
  state?: string;
  reconciling?: boolean;
  url?: string;
  uri?: string;
  updateTime?: string;
};

export default function ServerlessManagerDrawer({ 
  open, onClose, isBuildpacksEnabled, onEnableBuildpacks, missingPack, onInstallPack 
}: Props) {
  const navigate = useNavigate();
  const [tab, setTab] = useState(0);
  const [functions, setFunctions] = useState<SlsResource[]>([]);
  const [services, setServices] = useState<SlsResource[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deployDialogOpen, setDeployDialogOpen] = useState(false);
  const [deployForm, setDeployForm] = useState({
    type: 'function',
    name: '',
    runtime: 'nodejs22',
    entryPoint: 'handler',
    code: '',
    useEventTrigger: false,
    triggerBucket: ''
  });

  // Pre-fill code based on runtime
  useEffect(() => {
    // Only pre-fill if the name is empty (implying a brand new resource, not an edit)
    if (deployForm.name) return;

    if (deployForm.runtime.startsWith('nodejs')) {
      setDeployForm(prev => ({ 
        ...prev, 
        entryPoint: 'handler',
        code: 'exports.handler = (req, res) => {\n  res.send("Hello from Node.js on MiniSky!");\n};' 
      }));
    } else if (deployForm.runtime.startsWith('python')) {
      setDeployForm(prev => ({ 
        ...prev, 
        entryPoint: 'handler',
        code: 'def handler(request):\n    return "Hello from Python on MiniSky!"' 
      }));
    } else if (deployForm.runtime.startsWith('go')) {
      setDeployForm(prev => ({ 
        ...prev, 
        entryPoint: 'Handler',
        code: 'package function\n\nimport (\n\t"fmt"\n\t"net/http"\n)\n\nfunc Handler(w http.ResponseWriter, r *http.Request) {\n\tfmt.Fprint(w, "Hello from Go on MiniSky!")\n}' 
      }));
    }
  }, [deployForm.runtime, deployDialogOpen]);
  const [logDialogOpen, setLogDialogOpen] = useState(false);
  const [logContent, setLogContent] = useState('');
  const [activeLogResource, setActiveLogResource] = useState('');

  const fetchResources = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const [fRes, sRes] = await Promise.all([
        fetch('/api/manage/serverless/functions'),
        fetch('/api/manage/serverless/services')
      ]);
      
      if (fRes.ok) {
        const data = await fRes.json();
        setFunctions(data.functions || []);
      }
      if (sRes.ok) {
        const data = await sRes.json();
        setServices(data.services || []);
      }
    } catch (err) {
      setError('Failed to connect to Serverless API');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const handleDelete = async (type: 'functions' | 'services', fullName: string) => {
    const resourceName = fullName.split('/').pop() || fullName;
    if (!confirm(`Are you sure you want to deprovision ${resourceName}?`)) return;
    
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/serverless/${type}/${resourceName}`, { method: 'DELETE' });
      if (res.ok) {
        fetchResources();
      } else {
        const text = await res.text();
        alert(`Deletion failed: ${text}`);
      }
    } catch (err) {
      alert('Network error during deletion');
    } finally {
      setLoading(false);
    }
  };
  const handleAction = async (action: () => void | Promise<void>) => {
    setActionLoading(true);
    try {
      await action();
    } finally {
      setActionLoading(false);
    }
  };
  
  const handleEdit = (resource: any) => {
    const name = resource.name.split('/').pop() || '';
    setDeployForm({
      type: tab === 0 ? 'function' : 'service',
      name: name,
      runtime: resource.runtime || 'nodejs22',
      entryPoint: resource.entryPoint || 'handler',
      code: resource.sourceCode || '',
      useEventTrigger: !!resource.eventTrigger,
      triggerBucket: resource.eventTrigger?.resource?.split('/').pop() || ''
    });
    setDeployDialogOpen(true);
  };

  const handleDeploy = async () => {
    setLoading(true);
    setDeployDialogOpen(false);
    try {
      const res = await fetch('/api/manage/serverless/deploy', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...deployForm,
          project: 'local-dev-project',
          location: 'us-central1',
          eventTrigger: deployForm.useEventTrigger ? {
            eventType: 'google.storage.object.finalize',
            resource: `projects/_/buckets/${deployForm.triggerBucket}`
          } : undefined,
        })
      });
      if (res.ok) {
        fetchResources();
      } else {
        const text = await res.text();
        setError(`Deployment failed: ${text}`);
      }
    } catch (err) {
      setError('Network error during deployment');
    } finally {
      setLoading(false);
    }
  };
  const handleViewLogs = async (name: string) => {  
    const resourceName = name.split("/").pop() || name; 
    setActiveLogResource(resourceName); 
    setLogDialogOpen(true); 
    setLogContent("Loading logs..."); 
    fetchLogs(resourceName); 
  }; 
  const fetchLogs = async (name: string) => { 
    try { 
      const res = await fetch(`/api/manage/serverless/logs/${name}`); 
      if (res.ok) { 
        setLogContent(await res.text()); 
      } 
    } catch (err) { 
      console.error("Failed to fetch logs", err); 
    } 
  };

  useEffect(() => {
    if (open) {
      fetchResources();
      const timer = setInterval(() => fetchResources(true), 5000);
      return () => clearInterval(timer);
    }
  }, [open]);

  useEffect(() => {
    if (logDialogOpen && activeLogResource) {
      const timer = setInterval(() => fetchLogs(activeLogResource), 2000);
      return () => clearInterval(timer);
    }
  }, [logDialogOpen, activeLogResource]);

  const activeResources = tab === 0 ? functions : services;
  const resourceLabel = tab === 0 ? 'Functions' : 'Cloud Run Services';
  const resourceType = tab === 0 ? 'functions' : 'services';

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: '750px', p: 4, height: '100%', display: 'flex', flexDirection: 'column' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Box sx={{ p: 1, borderRadius: '8px', background: '#e8f0fe', color: '#1a73e8' }}>
              <CloudQueueIcon />
            </Box>
            <Typography variant="h5" sx={{ fontWeight: 500 }}>Serverless Console</Typography>
          </Box>
          <Box>
            <Button 
              variant="contained" 
              size="small" 
              startIcon={<RocketLaunchIcon />} 
              sx={{ mr: 2 }}
              onClick={() => setDeployDialogOpen(true)}
            >
              Deploy New
            </Button>
            <IconButton onClick={() => fetchResources()} disabled={loading} size="small" sx={{ mr: 1 }}>
              <RefreshIcon fontSize="small" />
            </IconButton>
            <IconButton onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </Box>
        </Box>

        <Typography variant="body2" sx={{ color: '#5f6368', mb: 3 }}>
          Low-latency orchestration of Cloud Run services and Cloud Functions using 
          <strong> Google Cloud Buildpacks</strong>.
        </Typography>

        {!isBuildpacksEnabled && (
          <Alert 
            severity="warning" 
            sx={{ mb: 3 }}
            action={
              <Button 
                color="inherit" 
                size="small" 
                disabled={actionLoading}
                onClick={() => handleAction(missingPack ? onInstallPack : onEnableBuildpacks)}
                startIcon={actionLoading ? <CircularProgress size={16} color="inherit" /> : null}
              >
                {missingPack ? 'Install Pack CLI' : 'Enable Buildpacks'}
              </Button>
            }
          >
            {missingPack 
              ? "'pack' CLI is missing. MiniSky cannot perform local Source-to-Image builds." 
              : "Buildpacks integration is currently disabled. Serverless resources will be simulated in-memory."}
          </Alert>
        )}

        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
          <Tabs value={tab} onChange={(_, v) => setTab(v)}>
            <Tab icon={<FunctionsIcon sx={{ fontSize: 18 }} />} iconPosition="start" label="Cloud Functions v2" />
            <Tab icon={<RocketLaunchIcon sx={{ fontSize: 18 }} />} iconPosition="start" label="Cloud Run Services" />
          </Tabs>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>{error}</Alert>
        )}

        <Box sx={{ flexGrow: 1, overflow: 'auto' }}>
          <Table size="small" stickyHeader>
            <TableHead>
              <TableRow>
                <TableCell>Resource Name</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Local URL</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {loading && activeResources.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 6 }}>
                    <CircularProgress size={24} />
                    <Typography variant="body2" sx={{ mt: 2, color: '#80868b' }}>Querying Docker shims...</Typography>
                  </TableCell>
                </TableRow>
              ) : activeResources.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 8 }}>
                    <BuildIcon sx={{ fontSize: 48, color: '#dadce0', mb: 2 }} />
                    <Typography variant="body1" sx={{ color: '#5f6368' }}>No active {resourceLabel} detected</Typography>
                    <Typography variant="caption" sx={{ color: '#80868b' }}>Deploy resources via Terraform, SDK, or CLI to see them here.</Typography>
                  </TableCell>
                </TableRow>
              ) : (
                activeResources.map((r) => {
                  const name = r.name.split('/').pop();
                  const status = r.state || (r.reconciling ? 'DEPLOYING' : 'ACTIVE');
                  const url = r.url || r.uri;
                  
                  return (
                    <TableRow key={r.name} hover sx={{ opacity: status === 'DEPLOYING' ? 0.7 : 1 }}>
                      <TableCell>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                          {tab === 0 ? <FunctionsIcon sx={{ color: '#1a73e8', fontSize: 18 }} /> : <RocketLaunchIcon sx={{ color: '#1a73e8', fontSize: 18 }} />}
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>{name}</Typography>
                        </Box>
                      </TableCell>
                      <TableCell>
                        <Chip 
                          label={status} 
                          size="small" 
                          sx={{ 
                            height: '20px', 
                            fontSize: '0.65rem', 
                            fontWeight: 700, 
                            backgroundColor: status === 'DEPLOYING' ? '#feefc3' : '#e6f4ea', 
                            color: status === 'DEPLOYING' ? '#ea8600' : '#1e8e3e',
                            animation: status === 'DEPLOYING' ? 'pulse 1.5s infinite' : 'none'
                          }} 
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace', color: '#1a73e8' }}>
                          {status === 'DEPLOYING' ? 'Building Image...' : (
                            <a href={url} target="_blank" rel="noreferrer" style={{ textDecoration: 'none', color: 'inherit' }}>
                              {url?.replace('http://', '')}
                            </a>
                          )}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="View Build Logs">
                          <IconButton size="small" sx={{ mr: 0.5 }} onClick={() => handleViewLogs(r.name)}>
                            <TerminalIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Edit & Redeploy">
                          <IconButton size="small" sx={{ mr: 0.5 }} onClick={() => handleEdit(r)}>
                            <BuildIcon fontSize="small" sx={{ color: '#1a73e8' }} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="View Runtime Logs">
                          <IconButton size="small" sx={{ mr: 0.5 }} onClick={() => { onClose(); navigate('/logging'); }}>
                            <MonitorHeartIcon fontSize="small" sx={{ color: '#81c995' }} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Deprovision Resource">
                          <IconButton size="small" color="error" onClick={() => handleDelete(resourceType, r.name)}>
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
        </Box>

        <Box sx={{ mt: 3, pt: 3, borderTop: '1px solid #dadce0', display: 'flex', gap: 2 }}>
          <Box sx={{ background: '#f8f9fa', p: 2, borderRadius: '8px', flex: 1 }}>
            <Typography variant="caption" sx={{ fontWeight: 600, color: '#3c4043', display: 'block', mb: 1 }}>
              S2I (Source-to-Image) Strategy
            </Typography>
            <Typography variant="caption" sx={{ color: '#5f6368', display: 'block' }}>
              Cloud Run and Cloud Functions (v2) in MiniSky use the same underlying Buildpacks engine. 
              The resulting containers are ephemeral but durable enough for local integration testing.
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

      <Dialog open={deployDialogOpen} onClose={() => setDeployDialogOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Deploy New Serverless Resource</DialogTitle>
        <DialogContent sx={{ pt: 2 }}>
          <Box sx={{ display: 'flex', gap: 2, mb: 3 }}>
            <FormControl fullWidth size="small">
              <InputLabel>Resource Type</InputLabel>
              <Select
                value={deployForm.type}
                label="Resource Type"
                onChange={(e) => setDeployForm({ ...deployForm, type: e.target.value })}
              >
                <MenuItem value="function">Cloud Function (v2)</MenuItem>
                <MenuItem value="service">Cloud Run Service</MenuItem>
              </Select>
            </FormControl>
            <FormControl fullWidth size="small">
              <InputLabel>Runtime</InputLabel>
              <Select
                value={deployForm.runtime}
                label="Runtime"
                onChange={(e) => setDeployForm({ ...deployForm, runtime: e.target.value })}
              >
                <MenuItem value="nodejs22">Node.js 22</MenuItem>
                <MenuItem value="nodejs20">Node.js 20</MenuItem>
                <MenuItem value="nodejs18">Node.js 18</MenuItem>
                <MenuItem value="python312">Python 3.12</MenuItem>
                <MenuItem value="python311">Python 3.11</MenuItem>
                <MenuItem value="python310">Python 3.10</MenuItem>
                <MenuItem value="go122">Go 1.22</MenuItem>
                <MenuItem value="go121">Go 1.21</MenuItem>
                <MenuItem value="go120">Go 1.20</MenuItem>
              </Select>
            </FormControl>
          </Box>
          <Box sx={{ display: 'flex', gap: 2, mb: 3 }}>
            <TextField
              fullWidth
              size="small"
              label="Resource Name"
              placeholder="e.g. hello-world"
              value={deployForm.name}
              onChange={(e) => setDeployForm({ ...deployForm, name: e.target.value })}
            />
            <TextField size="small" label="Entry Point" value={deployForm.entryPoint} onChange={e => setDeployForm({...deployForm, entryPoint: e.target.value})} fullWidth sx={{ mb: 2 }} />

            <Box sx={{ mb: 2, p: 2, border: '1px solid #444', borderRadius: 1 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                <input 
                  type="checkbox" 
                  checked={deployForm.useEventTrigger} 
                  onChange={e => setDeployForm({...deployForm, useEventTrigger: e.target.checked})} 
                  id="event-trigger-check" 
                />
                <label htmlFor="event-trigger-check" style={{ marginLeft: '8px', cursor: 'pointer', fontSize: '0.875rem' }}>Enable GCS Event Trigger</label>
              </Box>
              {deployForm.useEventTrigger && (
                <TextField 
                  size="small" 
                  label="Bucket Name" 
                  value={deployForm.triggerBucket} 
                  onChange={e => setDeployForm({...deployForm, triggerBucket: e.target.value})} 
                  fullWidth 
                  placeholder="e.g. my-bucket"
                />
              )}
            </Box>
          </Box>
          <Typography variant="caption" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>Source Code (entry point: index.js / main.py / function.go)</Typography>
          <TextField
            fullWidth
            multiline
            rows={10}
            variant="outlined"
            sx={{ '& .MuiInputBase-input': { fontFamily: 'monospace', fontSize: '0.85rem' } }}
            value={deployForm.code}
            onChange={(e) => setDeployForm({ ...deployForm, code: e.target.value })}
          />
        </DialogContent>
        <DialogActions sx={{ p: 3 }}>
          <Button onClick={() => setDeployDialogOpen(false)}>Cancel</Button>
          <Button 
            variant="contained" 
            onClick={handleDeploy} 
            disabled={!deployForm.name || !deployForm.code}
          >
            Deploy to MiniSky
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={logDialogOpen} onClose={() => setLogDialogOpen(false)} maxWidth="lg" fullWidth>
        <DialogTitle sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          Build Logs: {activeLogResource}
          <Chip label="Real-time" size="small" color="primary" sx={{ height: 20, fontSize: '0.65rem' }} />
        </DialogTitle>
        <DialogContent sx={{ backgroundColor: '#1e1e1e', color: '#d4d4d4', p: 0 }}>
          <Box sx={{ p: 2, fontFamily: 'monospace', fontSize: '0.85rem', whiteSpace: 'pre-wrap', maxHeight: '600px', overflow: 'auto' }}>
            {logContent || 'Waiting for build output...'}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setLogDialogOpen(false)} sx={{ color: '#1a73e8' }}>Close</Button>
        </DialogActions>
      </Dialog>
    </Drawer>
  );
}
