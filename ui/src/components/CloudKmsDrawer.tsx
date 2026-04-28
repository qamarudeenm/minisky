import { useState, useEffect } from 'react';
import {
  Drawer, Box, Typography, TextField, Button, Stack, Divider, Alert,
  List, ListItem, ListItemText, ListItemSecondaryAction, IconButton,
  CircularProgress, Tooltip, Collapse, Paper, Select, MenuItem,
  FormControl, InputLabel, Chip, Tabs, Tab
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import LockIcon from '@mui/icons-material/Lock';
import LockOpenIcon from '@mui/icons-material/LockOpen';
import KeyIcon from '@mui/icons-material/Key';
import { useProjectContext } from '../contexts/ProjectContext';

interface KeyRing {
  name: string;
  createTime: string;
}

interface CryptoKey {
  name: string;
  purpose: string;
  createTime: string;
  primary?: { name: string; state: string; algorithm: string };
}

interface CryptoKeyVersion {
  name: string;
  state: string;
  createTime: string;
  algorithm: string;
  destroyTime?: string;
}

interface Props {
  open: boolean;
  onClose: () => void;
}

const BASE = '/api/manage/cloudkms';
const LOCATION = 'global';

export default function CloudKmsDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [tab, setTab] = useState(0);
  const [error, setError] = useState<string | null>(null);

  // --- Key Rings ---
  const [keyRings, setKeyRings] = useState<KeyRing[]>([]);
  const [loadingKR, setLoadingKR] = useState(false);
  const [newKrId, setNewKrId] = useState('');
  const [creatingKR, setCreatingKR] = useState(false);

  // --- Crypto Keys ---
  const [selectedKr, setSelectedKr] = useState('');
  const [cryptoKeys, setCryptoKeys] = useState<CryptoKey[]>([]);
  const [loadingCK, setLoadingCK] = useState(false);
  const [newCkId, setNewCkId] = useState('');
  const [newCkPurpose, setNewCkPurpose] = useState('ENCRYPT_DECRYPT');
  const [creatingCK, setCreatingCK] = useState(false);

  // --- Key Versions ---
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const [versions, setVersions] = useState<CryptoKeyVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);

  // --- Encrypt/Decrypt Sandbox ---
  const [sandboxKr, setSandboxKr] = useState('');
  const [sandboxCk, setSandboxCk] = useState('');
  const [sandboxCkList, setSandboxCkList] = useState<CryptoKey[]>([]);
  const [plaintext, setPlaintext] = useState('');
  const [ciphertext, setCiphertext] = useState('');
  const [encryptResult, setEncryptResult] = useState('');
  const [decryptResult, setDecryptResult] = useState('');
  const [sandboxLoading, setSandboxLoading] = useState(false);

  const krBase = `${BASE}/projects/${activeProject}/locations/${LOCATION}/keyRings`;

  const fetchKeyRings = async () => {
    setLoadingKR(true);
    try {
      const res = await fetch(krBase);
      if (!res.ok) throw new Error('Failed to fetch key rings');
      const data = await res.json();
      setKeyRings(data.keyRings || []);
    } catch (e: any) { setError(e.message); }
    finally { setLoadingKR(false); }
  };

  const fetchCryptoKeys = async (krId: string) => {
    setLoadingCK(true);
    try {
      const res = await fetch(`${krBase}/${krId}/cryptoKeys`);
      if (!res.ok) throw new Error('Failed to fetch crypto keys');
      const data = await res.json();
      setCryptoKeys(data.cryptoKeys || []);
    } catch (e: any) { setError(e.message); }
    finally { setLoadingCK(false); }
  };

  const fetchVersions = async (krId: string, ckId: string) => {
    setLoadingVersions(true);
    try {
      const res = await fetch(`${krBase}/${krId}/cryptoKeys/${ckId}/cryptoKeyVersions`);
      if (!res.ok) throw new Error('Failed to fetch versions');
      const data = await res.json();
      setVersions(data.cryptoKeyVersions || []);
    } catch (e: any) { setError(e.message); }
    finally { setLoadingVersions(false); }
  };

  useEffect(() => {
    if (open) { fetchKeyRings(); setTab(0); }
    else { setSelectedKr(''); setCryptoKeys([]); setExpandedKey(null); }
  }, [open, activeProject]);

  useEffect(() => {
    if (selectedKr) { fetchCryptoKeys(selectedKr); setExpandedKey(null); }
  }, [selectedKr]);

  const handleCreateKeyRing = async () => {
    if (!newKrId.trim()) return;
    setCreatingKR(true); setError(null);
    try {
      const res = await fetch(`${krBase}?keyRingId=${newKrId}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
      if (!res.ok) { const d = await res.json(); throw new Error(d.error?.message || 'Failed'); }
      setNewKrId(''); fetchKeyRings();
    } catch (e: any) { setError(e.message); }
    finally { setCreatingKR(false); }
  };

  const handleCreateCryptoKey = async () => {
    if (!selectedKr || !newCkId.trim()) return;
    setCreatingCK(true); setError(null);
    try {
      const res = await fetch(`${krBase}/${selectedKr}/cryptoKeys?cryptoKeyId=${newCkId}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ purpose: newCkPurpose })
      });
      if (!res.ok) { const d = await res.json(); throw new Error(d.error?.message || 'Failed'); }
      setNewCkId(''); fetchCryptoKeys(selectedKr);
    } catch (e: any) { setError(e.message); }
    finally { setCreatingCK(false); }
  };

  const handleRotateKey = async (krId: string, ckId: string) => {
    try {
      const res = await fetch(`${krBase}/${krId}/cryptoKeys/${ckId}/cryptoKeyVersions`, { method: 'POST' });
      if (!res.ok) throw new Error('Failed to rotate key');
      fetchCryptoKeys(krId);
      if (expandedKey === ckId) fetchVersions(krId, ckId);
    } catch (e: any) { setError(e.message); }
  };

  const handleDestroyVersion = async (krId: string, ckId: string, versionName: string) => {
    const vId = versionName.split('/').pop();
    if (!confirm(`Destroy version ${vId}? This cannot be undone.`)) return;
    try {
      const res = await fetch(`${BASE}/${versionName}:destroy`, { method: 'POST' });
      if (!res.ok) throw new Error('Failed to destroy version');
      fetchVersions(krId, ckId);
    } catch (e: any) { setError(e.message); }
  };

  const toggleVersions = (krId: string, ckId: string) => {
    if (expandedKey === ckId) { setExpandedKey(null); setVersions([]); }
    else { setExpandedKey(ckId); fetchVersions(krId, ckId); }
  };

  // Sandbox handlers
  const handleSandboxKrChange = async (krId: string) => {
    setSandboxKr(krId); setSandboxCk(''); setSandboxCkList([]);
    if (!krId) return;
    try {
      const res = await fetch(`${krBase}/${krId}/cryptoKeys`);
      const data = await res.json();
      setSandboxCkList((data.cryptoKeys || []).filter((k: CryptoKey) => k.purpose === 'ENCRYPT_DECRYPT'));
    } catch {}
  };

  const handleEncrypt = async () => {
    if (!sandboxKr || !sandboxCk || !plaintext) return;
    setSandboxLoading(true); setEncryptResult(''); setError(null);
    try {
      const res = await fetch(`${krBase}/${sandboxKr}/cryptoKeys/${sandboxCk}:encrypt`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plaintext: btoa(plaintext) })
      });
      if (!res.ok) throw new Error('Encryption failed');
      const data = await res.json();
      setEncryptResult(data.ciphertext);
    } catch (e: any) { setError(e.message); }
    finally { setSandboxLoading(false); }
  };

  const handleDecrypt = async () => {
    if (!sandboxKr || !sandboxCk || !ciphertext) return;
    setSandboxLoading(true); setDecryptResult(''); setError(null);
    try {
      const res = await fetch(`${krBase}/${sandboxKr}/cryptoKeys/${sandboxCk}:decrypt`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ciphertext })
      });
      if (!res.ok) throw new Error('Decryption failed — invalid ciphertext or wrong key');
      const data = await res.json();
      setDecryptResult(atob(data.plaintext));
    } catch (e: any) { setError(e.message); }
    finally { setSandboxLoading(false); }
  };

  const stateColor = (s: string) => s === 'ENABLED' ? 'success' : s === 'DESTROYED' ? 'error' : 'default';

  return (
    <Drawer anchor="right" open={open} onClose={onClose} sx={{ '& .MuiDrawer-paper': { width: 600 } }}>
      <Box sx={{ p: 4, height: '100%', overflow: 'auto' }}>
        {/* Header */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <KeyIcon sx={{ color: '#4285f4' }} />
            <Typography variant="h5" sx={{ fontWeight: 600 }}>Cloud KMS</Typography>
          </Box>
          <IconButton onClick={onClose}><CloseIcon /></IconButton>
        </Box>
        <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
          Native AES-256-GCM key management. No external Docker container required.
        </Typography>
        <Divider sx={{ mb: 3 }} />

        {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>{error}</Alert>}

        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 3 }}>
          <Tab label="Key Rings & Keys" />
          <Tab label="Encrypt / Decrypt" />
        </Tabs>

        {/* ============ TAB 0: Key Rings & Keys ============ */}
        {tab === 0 && (
          <Stack spacing={3}>
            {/* Create Key Ring */}
            <Paper variant="outlined" sx={{ p: 3, bgcolor: '#f8f9fa' }}>
              <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Create Key Ring</Typography>
              <Stack direction="row" spacing={1}>
                <TextField size="small" label="Key Ring ID" fullWidth value={newKrId}
                  onChange={e => setNewKrId(e.target.value)} placeholder="e.g. my-key-ring" />
                <Button variant="contained" startIcon={<AddIcon />} onClick={handleCreateKeyRing}
                  disabled={creatingKR || !newKrId.trim()} sx={{ whiteSpace: 'nowrap' }}>
                  {creatingKR ? '...' : 'Create'}
                </Button>
              </Stack>
            </Paper>

            {/* Key Ring List */}
            <Box>
              <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                Key Rings ({keyRings.length})
              </Typography>
              {loadingKR ? <CircularProgress size={20} /> : (
                <List dense disablePadding>
                  {keyRings.length === 0 && (
                    <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                      No key rings yet. Create one above.
                    </Typography>
                  )}
                  {keyRings.map(kr => {
                    const krId = kr.name.split('/').pop()!;
                    const isSelected = selectedKr === krId;
                    return (
                      <ListItem key={kr.name} disablePadding sx={{
                        mb: 0.5, px: 2, py: 1, borderRadius: 1, cursor: 'pointer',
                        bgcolor: isSelected ? 'primary.50' : '#fff',
                        border: `1px solid ${isSelected ? '#4285f4' : '#e0e0e0'}`,
                        '&:hover': { bgcolor: '#f0f4ff' }
                      }} onClick={() => setSelectedKr(isSelected ? '' : krId)}>
                        <ListItemText
                          primary={<Typography sx={{ fontWeight: 600, fontSize: '0.9rem' }}>{krId}</Typography>}
                          secondary={`Created: ${new Date(kr.createTime).toLocaleString()}`}
                        />
                        <Chip label={isSelected ? 'Selected' : 'Select'} size="small"
                          color={isSelected ? 'primary' : 'default'} variant={isSelected ? 'filled' : 'outlined'} />
                      </ListItem>
                    );
                  })}
                </List>
              )}
            </Box>

            {/* Crypto Keys */}
            {selectedKr && (
              <Box>
                <Divider sx={{ mb: 2 }} />
                <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
                  Crypto Keys in <code>{selectedKr}</code>
                </Typography>

                {/* Create Crypto Key */}
                <Paper variant="outlined" sx={{ p: 3, bgcolor: '#f8f9fa', mb: 2 }}>
                  <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Create Crypto Key</Typography>
                  <Stack spacing={2}>
                    <Stack direction="row" spacing={1}>
                      <TextField size="small" label="Crypto Key ID" fullWidth value={newCkId}
                        onChange={e => setNewCkId(e.target.value)} placeholder="e.g. data-encryption-key" />
                      <FormControl size="small" sx={{ minWidth: 180 }}>
                        <InputLabel>Purpose</InputLabel>
                        <Select value={newCkPurpose} label="Purpose" onChange={e => setNewCkPurpose(e.target.value)}>
                          <MenuItem value="ENCRYPT_DECRYPT">ENCRYPT_DECRYPT</MenuItem>
                          <MenuItem value="ASYMMETRIC_SIGN">ASYMMETRIC_SIGN</MenuItem>
                          <MenuItem value="MAC">MAC</MenuItem>
                        </Select>
                      </FormControl>
                    </Stack>
                    <Button variant="contained" startIcon={<AddIcon />} onClick={handleCreateCryptoKey}
                      disabled={creatingCK || !newCkId.trim()}>
                      {creatingCK ? 'Creating...' : 'Create Crypto Key'}
                    </Button>
                  </Stack>
                </Paper>

                {loadingCK ? <CircularProgress size={20} /> : (
                  <List dense disablePadding>
                    {cryptoKeys.length === 0 && (
                      <Typography variant="body2" color="text.secondary" sx={{ py: 2, textAlign: 'center' }}>
                        No crypto keys in this key ring.
                      </Typography>
                    )}
                    {cryptoKeys.map(ck => {
                      const ckId = ck.name.split('/').pop()!;
                      const isExpanded = expandedKey === ckId;
                      return (
                        <Box key={ck.name} sx={{ mb: 1, border: '1px solid #e0e0e0', borderRadius: 1, overflow: 'hidden' }}>
                          <ListItem sx={{ px: 2 }}>
                            <IconButton size="small" onClick={() => toggleVersions(selectedKr, ckId)} sx={{ mr: 1 }}>
                              {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                            </IconButton>
                            <ListItemText
                              primary={<Typography sx={{ fontWeight: 600, fontSize: '0.9rem' }}>{ckId}</Typography>}
                              secondary={
                                <Box sx={{ display: 'flex', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
                                  <Chip label={ck.purpose} size="small" variant="outlined" />
                                  {ck.primary && <Chip label={ck.primary.state} size="small" color={stateColor(ck.primary.state)} />}
                                </Box>
                              }
                            />
                            <ListItemSecondaryAction>
                              <Tooltip title="Rotate Key (add new version)">
                                <IconButton size="small" onClick={() => handleRotateKey(selectedKr, ckId)}>
                                  <LockOpenIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                              <Tooltip title="Copy Resource Name">
                                <IconButton size="small" onClick={() => navigator.clipboard.writeText(ck.name)}>
                                  <ContentCopyIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                            </ListItemSecondaryAction>
                          </ListItem>

                          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                            <Box sx={{ px: 3, pb: 2, bgcolor: '#fafafa' }}>
                              <Typography variant="caption" sx={{ color: '#5f6368', display: 'block', mb: 1 }}>
                                KEY VERSIONS
                              </Typography>
                              {loadingVersions ? <CircularProgress size={14} /> : (
                                <Stack spacing={0.5}>
                                  {versions.map(v => {
                                    const vId = v.name.split('/').pop();
                                    return (
                                      <Box key={v.name} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', bgcolor: '#fff', border: '1px solid #eee', borderRadius: 1, px: 2, py: 1 }}>
                                        <Box>
                                          <Typography variant="body2" sx={{ fontWeight: 600 }}>v{vId}</Typography>
                                          <Typography variant="caption" color="text.secondary">{v.algorithm}</Typography>
                                        </Box>
                                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                          <Chip label={v.state} size="small" color={stateColor(v.state)} />
                                          {v.state === 'ENABLED' && (
                                            <Tooltip title="Destroy Version">
                                              <IconButton size="small" color="error" onClick={() => handleDestroyVersion(selectedKr, ckId, v.name)}>
                                                <DeleteIcon fontSize="inherit" />
                                              </IconButton>
                                            </Tooltip>
                                          )}
                                        </Box>
                                      </Box>
                                    );
                                  })}
                                </Stack>
                              )}
                            </Box>
                          </Collapse>
                        </Box>
                      );
                    })}
                  </List>
                )}
              </Box>
            )}
          </Stack>
        )}

        {/* ============ TAB 1: Encrypt / Decrypt Sandbox ============ */}
        {tab === 1 && (
          <Stack spacing={3}>
            <Alert severity="info" icon={<LockIcon />}>
              Operations are performed locally using AES-256-GCM. Select an <strong>ENCRYPT_DECRYPT</strong> key to begin.
            </Alert>

            <Stack direction="row" spacing={2}>
              <FormControl size="small" fullWidth>
                <InputLabel>Key Ring</InputLabel>
                <Select value={sandboxKr} label="Key Ring" onChange={e => handleSandboxKrChange(e.target.value)}>
                  {keyRings.map(kr => {
                    const id = kr.name.split('/').pop()!;
                    return <MenuItem key={id} value={id}>{id}</MenuItem>;
                  })}
                </Select>
              </FormControl>
              <FormControl size="small" fullWidth>
                <InputLabel>Crypto Key</InputLabel>
                <Select value={sandboxCk} label="Crypto Key" onChange={e => setSandboxCk(e.target.value)} disabled={!sandboxKr}>
                  {sandboxCkList.map(ck => {
                    const id = ck.name.split('/').pop()!;
                    return <MenuItem key={id} value={id}>{id}</MenuItem>;
                  })}
                </Select>
              </FormControl>
            </Stack>

            <Divider />

            {/* Encrypt */}
            <Paper variant="outlined" sx={{ p: 3, bgcolor: '#f0f4ff' }}>
              <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 700, display: 'flex', alignItems: 'center', gap: 1 }}>
                <LockIcon fontSize="small" /> Encrypt
              </Typography>
              <TextField label="Plaintext" fullWidth multiline rows={3} size="small" value={plaintext}
                onChange={e => setPlaintext(e.target.value)} placeholder="Enter text to encrypt..." sx={{ mb: 2, bgcolor: '#fff' }} />
              <Button variant="contained" onClick={handleEncrypt} disabled={sandboxLoading || !sandboxCk || !plaintext}>
                {sandboxLoading ? <CircularProgress size={16} /> : 'Encrypt'}
              </Button>
              {encryptResult && (
                <Box sx={{ mt: 2 }}>
                  <Typography variant="caption" color="text.secondary">CIPHERTEXT (base64)</Typography>
                  <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, mt: 0.5 }}>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace', bgcolor: '#fff', p: 1, borderRadius: 1, border: '1px solid #ddd', wordBreak: 'break-all', flex: 1, fontSize: '0.75rem' }}>
                      {encryptResult}
                    </Typography>
                    <Tooltip title="Copy ciphertext">
                      <IconButton size="small" onClick={() => navigator.clipboard.writeText(encryptResult)}>
                        <ContentCopyIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Box>
              )}
            </Paper>

            {/* Decrypt */}
            <Paper variant="outlined" sx={{ p: 3, bgcolor: '#f0fff4' }}>
              <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 700, display: 'flex', alignItems: 'center', gap: 1 }}>
                <LockOpenIcon fontSize="small" /> Decrypt
              </Typography>
              <TextField label="Ciphertext (base64)" fullWidth multiline rows={3} size="small" value={ciphertext}
                onChange={e => setCiphertext(e.target.value)} placeholder="Paste base64 ciphertext..." sx={{ mb: 2, bgcolor: '#fff' }} />
              <Button variant="contained" color="success" onClick={handleDecrypt} disabled={sandboxLoading || !sandboxCk || !ciphertext}>
                {sandboxLoading ? <CircularProgress size={16} /> : 'Decrypt'}
              </Button>
              {decryptResult && (
                <Box sx={{ mt: 2 }}>
                  <Typography variant="caption" color="text.secondary">PLAINTEXT</Typography>
                  <Typography variant="body2" sx={{ fontFamily: 'monospace', bgcolor: '#fff', p: 1, borderRadius: 1, border: '1px solid #ddd', mt: 0.5, wordBreak: 'break-all' }}>
                    {decryptResult}
                  </Typography>
                </Box>
              )}
            </Paper>
          </Stack>
        )}
      </Box>
    </Drawer>
  );
}
