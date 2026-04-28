import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  Paper, TextField, Chip, Stack, Select, MenuItem, FormControl, InputLabel,
  Avatar, CircularProgress, Tabs, Tab
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import SendIcon from '@mui/icons-material/Send';
import SettingsIcon from '@mui/icons-material/Settings';
import ChatIcon from '@mui/icons-material/Chat';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import PersonIcon from '@mui/icons-material/Person';
import PsychologyIcon from '@mui/icons-material/Psychology';
import RefreshIcon from '@mui/icons-material/Refresh';
import { useProjectContext } from '../contexts/ProjectContext';

type VertexAiDrawerProps = { open: boolean; onClose: () => void };

export default function VertexAiDrawer({ open, onClose }: VertexAiDrawerProps) {
  const { activeProject } = useProjectContext();
  const [tab, setTab] = useState(0);

  // Settings
  const [provider, setProvider] = useState('ollama');
  const [endpoint, setEndpoint] = useState('http://localhost:11434');
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState('llama3');
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [isFetchingModels, setIsFetchingModels] = useState(false);

  // Chat
  const [input, setInput] = useState('');
  const [messages, setMessages] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const apiRoot = `/api/manage/vertexai/v1`;

  const fetchModels = useCallback(async () => {
    setIsFetchingModels(true);
    try {
      const res = await fetch(`${apiRoot}/internal/models`);
      if (res.ok) {
        const data = await res.json();
        setAvailableModels(data.models || []);
        if (data.models?.length > 0 && !data.models.includes(model)) {
          setModel(data.models[0]);
        }
      }
    } catch (e) { console.error(e); }
    finally { setIsFetchingModels(false); }
  }, [apiRoot, model]);

  useEffect(() => {
    if (open) {
      fetchModels();
    }
  }, [open, fetchModels]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  const saveConfig = async () => {
    try {
      await fetch(`${apiRoot}/internal/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, endpoint, apiKey, model })
      });
      fetchModels();
    } catch (e) { console.error(e); }
  };

  const handleSend = async () => {
    if (!input || loading) return;
    const userMsg = { role: 'user', text: input };
    setMessages(prev => [...prev, userMsg]);
    const currentInput = input;
    setInput('');
    setLoading(true);

    try {
      const res = await fetch(`${apiRoot}/projects/${activeProject}/locations/us-central1/publishers/google/models/${model}:generateContent`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          contents: [
            {
              role: 'user',
              parts: [{ text: currentInput }]
            }
          ]
        })
      });

      if (res.ok) {
        const data = await res.json();
        const modelText = data.candidates?.[0]?.content?.parts?.[0]?.text || 'No response from model.';
        setMessages(prev => [...prev, { role: 'model', text: modelText }]);
      } else {
        const err = await res.text();
        setMessages(prev => [...prev, { role: 'error', text: `Error: ${err}` }]);
      }
    } catch (e: any) {
      setMessages(prev => [...prev, { role: 'error', text: `Connection Error: ${e.message}` }]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '85vw', maxWidth: 1000, bgcolor: '#f8f9fa' }}>
        
        <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <PsychologyIcon sx={{ color: '#1a73e8' }} />
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500 }}>Vertex AI Model Garden</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Local & Remote LLM Orchestration • {activeProject}</Typography>
            </Box>
          </Box>
          <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
        </Box>

        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Tab icon={<ChatIcon sx={{ fontSize: '1.1rem' }} />} iconPosition="start" label="Sandbox Chat" />
          <Tab icon={<SettingsIcon sx={{ fontSize: '1.1rem' }} />} iconPosition="start" label="Provider Settings" />
        </Tabs>

        <Box sx={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
          {tab === 0 ? (
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', bgcolor: 'white' }}>
              <Box ref={scrollRef} sx={{ flex: 1, p: 3, overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 2 }}>
                {messages.length === 0 && (
                  <Box sx={{ textAlign: 'center', mt: 10, color: '#9aa0a6' }}>
                    <SmartToyIcon sx={{ fontSize: 64, mb: 2, opacity: 0.2 }} />
                    <Typography variant="h6">Local AI Sandbox</Typography>
                    <Typography variant="body2">Start a conversation with your local model.</Typography>
                    <Box sx={{ mt: 2 }}>
                      <Chip label={`Using model: ${model}`} color="primary" variant="outlined" size="small" />
                    </Box>
                  </Box>
                )}
                {messages.map((m, i) => (
                  <Box key={i} sx={{ 
                    display: 'flex', 
                    gap: 2, 
                    alignSelf: m.role === 'user' ? 'flex-end' : 'flex-start',
                    maxWidth: '80%',
                    flexDirection: m.role === 'user' ? 'row-reverse' : 'row'
                  }}>
                    <Avatar sx={{ 
                      bgcolor: m.role === 'user' ? '#1a73e8' : (m.role === 'error' ? '#d93025' : '#e8eaed'),
                      color: m.role === 'model' ? '#1a73e8' : 'white',
                      width: 32, height: 32 
                    }}>
                      {m.role === 'user' ? <PersonIcon fontSize="small" /> : <SmartToyIcon fontSize="small" />}
                    </Avatar>
                    <Paper sx={{ 
                      p: 2, 
                      borderRadius: 2,
                      bgcolor: m.role === 'user' ? '#1a73e8' : '#f1f3f4',
                      color: m.role === 'user' ? 'white' : 'inherit',
                      boxShadow: 'none',
                      border: m.role === 'error' ? '1px solid #d93025' : 'none'
                    }}>
                      <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', lineHeight: 1.6 }}>{m.text}</Typography>
                    </Paper>
                  </Box>
                ))}
                {loading && (
                  <Box sx={{ display: 'flex', gap: 2 }}>
                    <Avatar sx={{ bgcolor: '#e8eaed', color: '#1a73e8', width: 32, height: 32 }}><SmartToyIcon fontSize="small" /></Avatar>
                    <Paper sx={{ p: 2, bgcolor: '#f1f3f4', boxShadow: 'none' }}>
                      <CircularProgress size={16} sx={{ mt: 0.5 }} />
                    </Paper>
                  </Box>
                )}
              </Box>

              <Box sx={{ p: 2, borderTop: '1px solid #dadce0', bgcolor: '#f8f9fa' }}>
                <Box sx={{ display: 'flex', gap: 1 }}>
                  <TextField 
                    fullWidth 
                    placeholder="Type a message to your model..."
                    multiline
                    maxRows={4}
                    value={input}
                    onChange={e => setInput(e.target.value)}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && !e.shiftKey) {
                        e.preventDefault();
                        handleSend();
                      }
                    }}
                    sx={{ bgcolor: 'white', borderRadius: 2 }}
                  />
                  <IconButton 
                    color="primary" 
                    onClick={handleSend} 
                    disabled={!input || loading}
                    sx={{ alignSelf: 'flex-end', mb: 0.5, bgcolor: 'white', p: 1.5 }}
                  >
                    <SendIcon />
                  </IconButton>
                </Box>
              </Box>
            </Box>
          ) : (
            <Box sx={{ flex: 1, p: 4, display: 'flex', flexDirection: 'column', gap: 4, bgcolor: 'white', overflow: 'auto' }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Model Provider Configuration</Typography>
              
              <Stack spacing={3} sx={{ maxWidth: 600 }}>
                <FormControl fullWidth>
                  <InputLabel>Backend Provider</InputLabel>
                  <Select value={provider} label="Backend Provider" onChange={e => setProvider(e.target.value)}>
                    <MenuItem value="ollama">Ollama (Local)</MenuItem>
                    <MenuItem value="openai">OpenAI / OpenRouter (Remote)</MenuItem>
                    <MenuItem value="llamacpp">llama.cpp / LM Studio (Local)</MenuItem>
                  </Select>
                </FormControl>

                <TextField 
                  label="Endpoint URL" 
                  fullWidth 
                  value={endpoint} 
                  onChange={e => setEndpoint(e.target.value)}
                  helperText={provider === 'ollama' ? "Usually http://localhost:11434" : "e.g. https://openrouter.ai/api/v1"}
                />

                <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
                  <FormControl fullWidth>
                    <InputLabel>Target Model</InputLabel>
                    <Select 
                      value={model} 
                      label="Target Model" 
                      onChange={e => setModel(e.target.value)}
                      disabled={isFetchingModels}
                    >
                      {availableModels.map(m => (
                        <MenuItem key={m} value={m}>{m}</MenuItem>
                      ))}
                      {availableModels.length === 0 && (
                        <MenuItem value={model} disabled>{model} (Current)</MenuItem>
                      )}
                    </Select>
                  </FormControl>
                  <IconButton onClick={fetchModels} disabled={isFetchingModels}>
                    <RefreshIcon className={isFetchingModels ? 'spin' : ''} />
                  </IconButton>
                </Box>

                <TextField 
                  label="API Key" 
                  type="password"
                  fullWidth 
                  value={apiKey} 
                  onChange={e => setApiKey(e.target.value)}
                  placeholder={provider === 'ollama' ? "Not required for Ollama" : "sk-..."}
                />

                <Button variant="contained" size="large" onClick={saveConfig} sx={{ mt: 2 }}>
                  Apply Configuration
                </Button>
              </Stack>

              <Box sx={{ height: '1px', bgcolor: '#dadce0', my: 2 }} />

              <Box>
                <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>Vertex AI Compatibility Note</Typography>
                <Typography variant="body2" sx={{ color: '#5f6368' }}>
                  MiniSky maps the standard Google Vertex AI API (`generateContent`) to your local backend. 
                  This allows you to use the official Google Cloud SDKs in your code while transparently talking to a local LLM.
                </Typography>
              </Box>
            </Box>
          )}
        </Box>
      </Box>
      <style>{`
        @keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
        .spin { animation: spin 1s linear infinite; }
      `}</style>
    </Drawer>
  );
}
