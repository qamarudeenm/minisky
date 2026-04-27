import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box, Typography, Button, Drawer, IconButton, Table, TableBody, TableCell,
  TableHead, TableRow, Chip, Alert, CircularProgress, Tooltip, Tabs, Tab,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, MenuItem,
  Select, FormControl, InputLabel,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import TerminalIcon from '@mui/icons-material/Terminal';
import LayersIcon from '@mui/icons-material/Layers';
import MiscellaneousServicesIcon from '@mui/icons-material/MiscellaneousServices';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutlined';
import PauseCircleOutlineIcon from '@mui/icons-material/PauseCircleOutlined';

type Props = { open: boolean; onClose: () => void };

type AEService = { id: string; name: string };
type AEVersion = {
  id: string;
  name: string;
  runtime: string;
  servingStatus: string;
  createTime: string;
};

const DEFAULT_CODE: Record<string, string> = {
  'python312':
    'from flask import Flask\napp = Flask(__name__)\n\n@app.route("/")\ndef hello():\n    return "Hello from App Engine on MiniSky!"',
  'nodejs22':
    'const express = require("express");\nconst app = express();\n\napp.get("/", (req, res) => {\n  res.send("Hello from App Engine on MiniSky!");\n});\n\napp.listen(process.env.PORT || 8080);',
  'go122':
    'package main\n\nimport (\n\t"fmt"\n\t"net/http"\n\t"os"\n)\n\nfunc main() {\n\tport := os.Getenv("PORT")\n\tif port == "" {\n\t\tport = "8080"\n\t}\n\thttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {\n\t\tfmt.Fprint(w, "Hello from App Engine on MiniSky!")\n\t})\n\thttp.ListenAndServe(":"+port, nil)\n}',
};

const RUNTIMES = [
  'python312', 'python311', 'python310',
  'nodejs22', 'nodejs20', 'nodejs18',
  'go122', 'go121',
];

function statusChip(status: string) {
  const serving = status === 'SERVING';
  return (
    <Chip
      size="small"
      icon={serving
        ? <CheckCircleOutlineIcon sx={{ fontSize: '14px !important' }} />
        : <PauseCircleOutlineIcon sx={{ fontSize: '14px !important' }} />}
      label={status}
      sx={{
        height: 22,
        fontSize: '0.65rem',
        fontWeight: 700,
        backgroundColor: serving ? '#e6f4ea' : '#fce8e6',
        color: serving ? '#1e8e3e' : '#c5221f',
        '& .MuiChip-icon': { color: serving ? '#1e8e3e' : '#c5221f' },
      }}
    />
  );
}

export default function AppEngineManagerDrawer({ open, onClose }: Props) {
  const navigate = useNavigate();
  const [tab, setTab] = useState(0);
  const [services, setServices] = useState<AEService[]>([]);
  const [versions, setVersions] = useState<AEVersion[]>([]);
  const [selectedService, setSelectedService] = useState('default');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deployOpen, setDeployOpen] = useState(false);
  const [opPoll, setOpPoll] = useState<string | null>(null);

  const [form, setForm] = useState({
    service: 'default',
    version: '',
    runtime: 'python312',
    entrypoint: 'gunicorn -b :$PORT main:app',
    code: DEFAULT_CODE['python312'],
  });

  // Sync default code when runtime changes
  useEffect(() => {
    if (!form.code || Object.values(DEFAULT_CODE).includes(form.code)) {
      setForm(f => ({ ...f, code: DEFAULT_CODE[f.runtime] ?? f.code, entrypoint: f.runtime.startsWith('go') ? '' : f.runtime.startsWith('nodejs') ? 'node index.js' : 'gunicorn -b :$PORT main:app' }));
    }
  }, [form.runtime]);

  const { activeProject } = useProjectContext();

  const fetchServices = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/manage/appengine/projects/${activeProject}/services`);
      if (res.ok) {
        const data = await res.json();
        setServices(data.services || []);
      }
    } catch {
      if (!silent) setError('Failed to reach App Engine API');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const fetchVersions = async (svc = selectedService, silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await fetch(`/api/manage/appengine/projects/${activeProject}/services/${svc}/versions`);
      if (res.ok) {
        const data = await res.json();
        setVersions(data.versions || []);
      }
    } catch {
      /* ignore */
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const pollOperation = async (opName: string) => {
    setOpPoll(opName);
    const interval = setInterval(async () => {
      try {
        const res = await fetch(`/api/manage/appengine/projects/${activeProject}/operations/${opName}`);
        if (res.ok) {
          const op = await res.json();
          if (op.done) {
            clearInterval(interval);
            setOpPoll(null);
            fetchVersions();
            fetchServices();
          }
        }
      } catch { /* ignore */ }
    }, 2000);
    // Stop polling after 2 min regardless
    setTimeout(() => { clearInterval(interval); setOpPoll(null); }, 120_000);
  };

  useEffect(() => {
    if (open) {
      fetchServices();
      fetchVersions();
      const t = setInterval(() => { fetchServices(true); fetchVersions(selectedService, true); }, 5000);
      return () => clearInterval(t);
    }
  }, [open, selectedService]);

  const handleDeploy = async () => {
    setDeployOpen(false);
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/appengine/projects/${activeProject}/deploy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          project: activeProject,
          service: form.service || 'default',
          version: form.version || undefined,
          runtime: form.runtime,
          entrypoint: form.entrypoint,
          code: form.code,
        }),
      });
      if (res.ok) {
        const op = await res.json();
        const opId = op.name?.split('/').pop();
        if (opId) pollOperation(opId);
        else { fetchVersions(); fetchServices(); }
      } else {
        const txt = await res.text();
        setError(`Deploy failed: ${txt}`);
      }
    } catch (e: any) {
      setError(`Network error: ${e.message}`);
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteVersion = async (versionId: string) => {
    if (!confirm(`Delete version "${versionId}"?`)) return;
    setLoading(true);
    try {
      const res = await fetch(
        `/api/manage/appengine/projects/${activeProject}/services/${selectedService}/versions/${versionId}`,
        { method: 'DELETE' }
      );
      if (res.ok) fetchVersions();
      else setError('Delete failed');
    } catch {
      setError('Network error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 780, p: 4, height: '100%', display: 'flex', flexDirection: 'column' }}>

        {/* ── Header ── */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Box sx={{ p: 1, borderRadius: '8px', background: '#e8f0fe', color: '#1a73e8' }}>
              <RocketLaunchIcon />
            </Box>
            <Box>
              <Typography variant="h5" sx={{ fontWeight: 500, lineHeight: 1.2 }}>
                App Engine
              </Typography>
                {activeProject}.appspot.com
            </Box>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {opPoll && (
              <Chip
                label="Deploying…"
                size="small"
                icon={<CircularProgress size={10} color="inherit" />}
                sx={{ height: 22, fontSize: '0.65rem', fontWeight: 700, backgroundColor: '#feefc3', color: '#ea8600' }}
              />
            )}
            <Button
              variant="contained"
              size="small"
              startIcon={<RocketLaunchIcon />}
              onClick={() => setDeployOpen(true)}
              disabled={!!opPoll}
            >
              Deploy
            </Button>
            <IconButton onClick={() => { fetchServices(); fetchVersions(); }} disabled={loading} size="small">
              <RefreshIcon fontSize="small" />
            </IconButton>
            <IconButton onClick={onClose}><CloseIcon /></IconButton>
          </Box>
        </Box>

        <Typography variant="body2" sx={{ color: '#5f6368', mb: 2 }}>
          Managed PaaS environment — deploy versioned services and route traffic without managing infrastructure.
        </Typography>

        {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

        {/* ── Tabs ── */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
          <Tabs value={tab} onChange={(_, v) => setTab(v)}>
            <Tab icon={<MiscellaneousServicesIcon sx={{ fontSize: 17 }} />} iconPosition="start" label="Services" />
            <Tab icon={<LayersIcon sx={{ fontSize: 17 }} />} iconPosition="start" label="Versions" />
          </Tabs>
        </Box>

        {/* ── Services Tab ── */}
        {tab === 0 && (
          <Box sx={{ flexGrow: 1, overflow: 'auto' }}>
            <Table size="small" stickyHeader>
              <TableHead>
                <TableRow>
                  <TableCell>Service ID</TableCell>
                  <TableCell>Resource Name</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {loading && services.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3} align="center" sx={{ py: 6 }}>
                      <CircularProgress size={24} />
                    </TableCell>
                  </TableRow>
                ) : services.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={3} align="center" sx={{ py: 8 }}>
                      <MiscellaneousServicesIcon sx={{ fontSize: 48, color: '#dadce0', mb: 1 }} />
                      <Typography variant="body2" sx={{ color: '#5f6368' }}>No services deployed yet</Typography>
                      <Typography variant="caption" sx={{ color: '#9aa0a6' }}>
                        Deploy a version to create your first service.
                      </Typography>
                    </TableCell>
                  </TableRow>
                ) : (
                  services.map(s => (
                    <TableRow
                      key={s.id}
                      hover
                      sx={{ cursor: 'pointer' }}
                      onClick={() => { setSelectedService(s.id); setTab(1); fetchVersions(s.id); }}
                    >
                      <TableCell>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <MiscellaneousServicesIcon sx={{ fontSize: 16, color: '#1a73e8' }} />
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>{s.id}</Typography>
                          {s.id === 'default' && (
                            <Chip label="default" size="small" sx={{ height: 18, fontSize: '0.6rem', backgroundColor: '#e8f0fe', color: '#1a73e8' }} />
                          )}
                        </Box>
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" sx={{ fontFamily: 'monospace', color: '#5f6368' }}>{s.name}</Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="View Versions">
                          <IconButton size="small" onClick={e => { e.stopPropagation(); setSelectedService(s.id); setTab(1); fetchVersions(s.id); }}>
                            <LayersIcon fontSize="small" sx={{ color: '#1a73e8' }} />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="View Logs">
                          <IconButton size="small" onClick={e => { e.stopPropagation(); onClose(); navigate('/logging'); }}>
                            <TerminalIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </Box>
        )}

        {/* ── Versions Tab ── */}
        {tab === 1 && (
          <Box sx={{ flexGrow: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
            {/* Service selector */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2 }}>
              <FormControl size="small" sx={{ minWidth: 200 }}>
                <InputLabel>Service</InputLabel>
                <Select
                  value={selectedService}
                  label="Service"
                  onChange={e => { setSelectedService(e.target.value); fetchVersions(e.target.value); }}
                >
                  {services.length === 0
                    ? <MenuItem value="default">default</MenuItem>
                    : services.map(s => <MenuItem key={s.id} value={s.id}>{s.id}</MenuItem>)
                  }
                </Select>
              </FormControl>
              <Typography variant="caption" sx={{ color: '#9aa0a6' }}>
                {versions.length} version{versions.length !== 1 ? 's' : ''}
              </Typography>
            </Box>

            <Table size="small" stickyHeader>
              <TableHead>
                <TableRow>
                  <TableCell>Version ID</TableCell>
                  <TableCell>Runtime</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Created</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {loading && versions.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} align="center" sx={{ py: 6 }}>
                      <CircularProgress size={24} />
                      <Typography variant="caption" sx={{ display: 'block', mt: 1, color: '#80868b' }}>
                        Loading versions…
                      </Typography>
                    </TableCell>
                  </TableRow>
                ) : versions.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} align="center" sx={{ py: 8 }}>
                      <LayersIcon sx={{ fontSize: 48, color: '#dadce0', mb: 1 }} />
                      <Typography variant="body2" sx={{ color: '#5f6368' }}>No versions for "{selectedService}"</Typography>
                      <Typography variant="caption" sx={{ color: '#9aa0a6' }}>
                        Click Deploy to push your first version.
                      </Typography>
                    </TableCell>
                  </TableRow>
                ) : (
                  versions.map((v, idx) => {
                    const isLatest = idx === 0;
                    return (
                      <TableRow key={v.id} hover>
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontWeight: 500 }}>{v.id}</Typography>
                            {isLatest && (
                              <Chip label="latest" size="small" sx={{ height: 18, fontSize: '0.6rem', backgroundColor: '#e6f4ea', color: '#1e8e3e' }} />
                            )}
                          </Box>
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption" sx={{ fontFamily: 'monospace', color: '#3c4043' }}>{v.runtime}</Typography>
                        </TableCell>
                        <TableCell>{statusChip(v.servingStatus)}</TableCell>
                        <TableCell>
                          <Typography variant="caption" sx={{ color: '#5f6368' }}>
                            {v.createTime ? new Date(v.createTime).toLocaleString() : '—'}
                          </Typography>
                        </TableCell>
                        <TableCell align="right">
                          <Tooltip title="View Logs">
                            <IconButton size="small" sx={{ mr: 0.5 }} onClick={() => { onClose(); navigate('/logging'); }}>
                              <TerminalIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Delete Version">
                            <IconButton size="small" color="error" onClick={() => handleDeleteVersion(v.id)}>
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
        )}

        {/* ── Footer info ── */}
        <Box sx={{ mt: 2, pt: 2, borderTop: '1px solid #dadce0' }}>
          <Box sx={{ background: '#f8f9fa', p: 2, borderRadius: '8px' }}>
            <Typography variant="caption" sx={{ fontWeight: 600, color: '#3c4043', display: 'block', mb: 0.5 }}>
              MiniSky App Engine Runtime
            </Typography>
            <Typography variant="caption" sx={{ color: '#5f6368' }}>
              Versions are packaged via Google Cloud Buildpacks and run as isolated Docker containers.
              Traffic is automatically routed to the latest SERVING version.
            </Typography>
          </Box>
        </Box>
      </Box>

      {/* ── Deploy Dialog ── */}
      <Dialog open={deployOpen} onClose={() => setDeployOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle sx={{ pb: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <RocketLaunchIcon sx={{ color: '#1a73e8' }} />
            Deploy New Version
          </Box>
        </DialogTitle>
        <DialogContent sx={{ pt: 2 }}>
          {/* Row 1: Service + Runtime */}
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth
              size="small"
              label="Service Name"
              placeholder="default"
              value={form.service}
              onChange={e => setForm({ ...form, service: e.target.value })}
            />
            <FormControl fullWidth size="small">
              <InputLabel>Runtime</InputLabel>
              <Select value={form.runtime} label="Runtime" onChange={e => setForm({ ...form, runtime: e.target.value })}>
                {RUNTIMES.map(r => <MenuItem key={r} value={r}>{r}</MenuItem>)}
              </Select>
            </FormControl>
          </Box>
          {/* Row 2: Version + Entrypoint */}
          <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
            <TextField
              fullWidth
              size="small"
              label="Version ID (optional)"
              placeholder="auto-generated"
              value={form.version}
              onChange={e => setForm({ ...form, version: e.target.value })}
            />
            <TextField
              fullWidth
              size="small"
              label="Entrypoint"
              placeholder="e.g. gunicorn -b :$PORT main:app"
              value={form.entrypoint}
              onChange={e => setForm({ ...form, entrypoint: e.target.value })}
            />
          </Box>
          {/* Code editor */}
          <Typography variant="caption" sx={{ fontWeight: 600, mb: 0.5, display: 'block', color: '#3c4043' }}>
            Source Code
          </Typography>
          <TextField
            fullWidth
            multiline
            rows={12}
            variant="outlined"
            sx={{ '& .MuiInputBase-input': { fontFamily: 'monospace', fontSize: '0.82rem' } }}
            value={form.code}
            onChange={e => setForm({ ...form, code: e.target.value })}
          />
        </DialogContent>
        <DialogActions sx={{ p: 2.5 }}>
          <Button onClick={() => setDeployOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<RocketLaunchIcon />}
            onClick={handleDeploy}
            disabled={!form.code}
          >
            Deploy to App Engine
          </Button>
        </DialogActions>
      </Dialog>

      <style>{`
        @keyframes pulse {
          0% { opacity: 1; } 50% { opacity: 0.5; } 100% { opacity: 1; }
        }
      `}</style>
    </Drawer>
  );
}
