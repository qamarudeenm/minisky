import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText, TextField,
  Snackbar, Alert, Dialog,
  DialogTitle, DialogContent, DialogActions, Paper, Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import SendIcon from '@mui/icons-material/Send';
import DownloadIcon from '@mui/icons-material/Download';
import RefreshIcon from '@mui/icons-material/Refresh';
import { useProjectContext } from '../contexts/ProjectContext';

type PubSubManagerDrawerProps = { open: boolean; onClose: () => void };

export default function PubSubManagerDrawer({ open, onClose }: PubSubManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [topics, setTopics] = useState<any[]>([]);
  const [activeTopic, setActiveTopic] = useState<string | null>(null);
  
  const [subscriptions, setSubscriptions] = useState<any[]>([]);
  const [activeSubscription, setActiveSubscription] = useState<string | null>(null);

  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Dialogs
  const [newTopicOpen, setNewTopicOpen] = useState(false);
  const [newColName, setNewColName] = useState('');
  
  const [newSubOpen, setNewSubOpen] = useState(false);
  const [newSubName, setNewSubName] = useState('');

  // Sandbox State
  const [publishPayload, setPublishPayload] = useState('{\n  "hello": "world"\n}');
  const [pulledMessages, setPulledMessages] = useState<any[]>([]);
  
  const apiRoot = `/api/manage/pubsub/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadTopics = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/topics`);
      if (res.ok) {
        const data = await res.json();
        setTopics(data.topics || []);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  const loadSubscriptions = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/subscriptions`);
      if (res.ok) {
        const data = await res.json();
        setSubscriptions(data.subscriptions || []);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) {
      loadTopics();
      loadSubscriptions();
    } else {
      setActiveTopic(null);
      setActiveSubscription(null);
    }
  }, [open, loadTopics, loadSubscriptions]);

  // When active topic changes, we unselect sub and fetch its subs
  useEffect(() => {
    setActiveSubscription(null);
    setPulledMessages([]);
  }, [activeTopic]);

  const handleCreateTopic = async () => {
    if (!newColName) return;
    try {
      const res = await fetch(`${apiRoot}/topics/${newColName}`, { method: 'PUT' });
      if (res.ok) {
        showToast('Topic created');
        loadTopics();
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewTopicOpen(false);
    setNewColName('');
  };

  const handleCreateSub = async () => {
    if (!newSubName || !activeTopic) return;
    try {
      const res = await fetch(`${apiRoot}/subscriptions/${newSubName}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ topic: activeTopic })
      });
      if (res.ok) {
        showToast('Subscription created');
        loadSubscriptions();
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewSubOpen(false);
    setNewSubName('');
  };

  const handleDeleteTopic = async (name: string) => {
    await fetch(`/api/manage/pubsub/${name}`, { method: 'DELETE' });
    if (activeTopic === name) setActiveTopic(null);
    loadTopics();
  };

  const handleDeleteSub = async (name: string) => {
    await fetch(`/api/manage/pubsub/${name}`, { method: 'DELETE' });
    if (activeSubscription === name) setActiveSubscription(null);
    loadSubscriptions();
  };

  const handlePublish = async () => {
    if (!activeTopic) return;
    setLoading(true);
    try {
      const dataStr = publishPayload;
      const base64Data = btoa(unescape(encodeURIComponent(dataStr))); // Safely base64 encode utf8 payload
      
      const res = await fetch(`/api/manage/pubsub/${activeTopic}:publish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: [{ data: base64Data }] })
      });
      if (res.ok) {
        showToast('Message published successfully to topic');
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed to publish', 'error');
      }
    } catch (e: any) {
      showToast('Error publishing: ' + e.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handlePull = async () => {
    if (!activeSubscription) return;
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/pubsub/${activeSubscription}:pull`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ returnImmediately: true, maxMessages: 10 })
      });
      if (res.ok) {
        const data = await res.json();
        if (data.receivedMessages && data.receivedMessages.length > 0) {
          const decoded = data.receivedMessages.map((msg: any) => {
            let text = '';
            try { text = decodeURIComponent(escape(atob(msg.message.data))); } catch(err) { text = '<binary>'; }
            return {
              id: msg.message.messageId,
              publishTime: msg.message.publishTime,
              data: text,
              ackId: msg.ackId
            };
          });
          setPulledMessages(decoded);
          showToast(`Pulled ${decoded.length} messages`);

          // Auto-ack to avoid pulling same messages immediately again (common emulator workflow)
          const ackIds = data.receivedMessages.map((m: any) => m.ackId);
          fetch(`/api/manage/pubsub/${activeSubscription}:acknowledge`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ackIds })
          });
        } else {
          showToast('No new messages available', 'error');
          setPulledMessages([]);
        }
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed to pull', 'error');
      }
    } catch (e: any) {
      showToast('Error pulling: ' + e.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  // Filter subs for active topic
  const filteredSubs = subscriptions.filter(s => s.topic === activeTopic);

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '85vw', maxWidth: 1200, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#f9ab00' }}>Cloud Pub/Sub</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Event Stream Sandbox • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => { loadTopics(); loadSubscriptions(); }} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            {/* Left Pane - Topics */}
            <Box sx={{ width: 280, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Topics</Typography>
                <IconButton size="small" onClick={() => setNewTopicOpen(true)}><AddIcon fontSize="small" /></IconButton>
              </Box>
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {topics.map(t => {
                  const tId = t.name.split('/').pop();
                  return (
                    <ListItemButton 
                      key={t.name} 
                      selected={activeTopic === t.name}
                      onClick={() => setActiveTopic(t.name)}
                      sx={{ borderBottom: '1px solid #f1f3f4', py: 1.5 }}
                    >
                      <ListItemText primary={<Typography sx={{ fontSize: '0.85rem', fontWeight: activeTopic === t.name ? 600 : 400 }}>{tId}</Typography>} />
                      <IconButton size="small" sx={{ opacity: 0.6, '&:hover': { opacity: 1, color: 'error.main' } }} onClick={(e) => { e.stopPropagation(); handleDeleteTopic(t.name); }}>
                        <DeleteIcon fontSize="inherit" />
                      </IconButton>
                    </ListItemButton>
                  );
                })}
              </List>
            </Box>

            {/* Middle Pane - Subscriptions */}
            <Box sx={{ width: 300, borderRight: '1px solid #dadce0', bgcolor: '#fff', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Subscriptions</Typography>
                <IconButton size="small" onClick={() => setNewSubOpen(true)} disabled={!activeTopic} color="primary">
                  <AddIcon fontSize="small" />
                </IconButton>
              </Box>
              
              {!activeTopic ? (
                <Typography variant="caption" sx={{ p: 2, color: '#80868b', fontStyle: 'italic', display: 'block' }}>
                  Select a topic to view its subscriptions.
                </Typography>
              ) : (
                <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                  {filteredSubs.length === 0 && (
                    <Typography variant="caption" sx={{ p: 2, color: '#80868b', fontStyle: 'italic', display: 'block' }}>
                      No subscriptions attached.
                    </Typography>
                  )}
                  {filteredSubs.map(s => {
                    const sId = s.name.split('/').pop();
                    return (
                      <ListItemButton 
                        key={s.name} 
                        selected={activeSubscription === s.name}
                        onClick={() => setActiveSubscription(s.name)}
                        sx={{ borderBottom: '1px solid #f1f3f4', py: 1.5 }}
                      >
                        <ListItemText 
                          primary={<Typography sx={{ fontSize: '0.85rem', fontWeight: activeSubscription === s.name ? 600 : 400 }}>{sId}</Typography>} 
                        />
                        <IconButton size="small" sx={{ opacity: 0.6, '&:hover': { opacity: 1, color: 'error.main' } }} onClick={(e) => { e.stopPropagation(); handleDeleteSub(s.name); }}>
                          <DeleteIcon fontSize="inherit" />
                        </IconButton>
                      </ListItemButton>
                    );
                  })}
                </List>
              )}
            </Box>

            {/* Right Pane - Sandbox */}
            <Box sx={{ flex: 1, p: 3, display: 'flex', flexDirection: 'column', bgcolor: '#f8f9fa', gap: 3, overflow: 'auto' }}>
              
              {/* Publish Sandbox */}
              <Box>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Publish Message Sandbox</Typography>
                  <Button variant="contained" size="small" startIcon={<SendIcon />} onClick={handlePublish} disabled={!activeTopic || loading}>
                    Publish to Topic
                  </Button>
                </Box>
                <Typography variant="caption" sx={{ color: '#5f6368', mb: 1, display: 'block' }}>
                  Type your raw string or JSON payload below. The UI will automatically Base64-encode it before streaming to the emulation backend.
                </Typography>
                <Paper elevation={0} sx={{ height: 180, display: 'flex' }}>
                  <TextField
                    multiline
                    fullWidth
                    variant="outlined"
                    value={publishPayload}
                    onChange={e => setPublishPayload(e.target.value)}
                    sx={{ height: '100%', '& .MuiInputBase-root': { height: '100%', alignItems: 'flex-start', fontFamily: 'monospace', fontSize: '0.85rem' } }}
                  />
                </Paper>
              </Box>

              {/* Pull Sandbox */}
              <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Subscriber Pull Sandbox</Typography>
                  <Button variant="outlined" size="small" startIcon={<DownloadIcon />} onClick={handlePull} disabled={!activeSubscription || loading}>
                    Fetch Messages
                  </Button>
                </Box>
                <Typography variant="caption" sx={{ color: '#5f6368', mb: 1, display: 'block' }}>
                  Locally queue messages bound to the selected subscription. Messages are auto-acknowledged upon pull.
                </Typography>
                <Paper elevation={0} sx={{ flex: 1, p: 2, overflow: 'auto', bgcolor: '#fff', border: '1px solid #dadce0' }}>
                  {pulledMessages.length === 0 ? (
                    <Typography variant="body2" sx={{ color: '#80868b', fontStyle: 'italic', textAlign: 'center', mt: 4 }}>
                      Hit "Fetch Messages" to pull events from the topic stream.
                    </Typography>
                  ) : (
                    pulledMessages.map(msg => (
                      <Box key={msg.id} sx={{ mb: 2, p: 1.5, bgcolor: '#f1f3f4', borderRadius: 1 }}>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                          <Chip label={`ID: ${msg.id}`} size="small" sx={{ fontSize: '0.6rem', height: 20 }} />
                          <Typography variant="caption" sx={{ color: '#5f6368' }}>{new Date(msg.publishTime).toLocaleTimeString()}</Typography>
                        </Box>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                          {msg.data}
                        </Typography>
                      </Box>
                    ))
                  )}
                </Paper>
              </Box>

            </Box>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newTopicOpen} onClose={() => setNewTopicOpen(false)}>
        <DialogTitle>Create New Topic</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Topic ID" fullWidth variant="standard"
                     value={newColName} onChange={e => setNewColName(e.target.value.replace(/[^a-zA-Z0-9_-]/g, ''))} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewTopicOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateTopic} disabled={!newColName}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newSubOpen} onClose={() => setNewSubOpen(false)}>
        <DialogTitle>Attach Subscription to {activeTopic?.split('/').pop()}</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Subscription ID" fullWidth variant="standard"
                     value={newSubName} onChange={e => setNewSubName(e.target.value.replace(/[^a-zA-Z0-9_-]/g, ''))} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewSubOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateSub} disabled={!newSubName}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
