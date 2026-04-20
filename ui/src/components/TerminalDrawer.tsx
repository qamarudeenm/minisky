import { useEffect, useRef } from 'react';
import { Drawer, Box, Typography, IconButton } from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

type TerminalDrawerProps = {
  open: boolean;
  onClose: () => void;
  containerName: string;
};

export default function TerminalDrawer({ open, onClose, containerName }: TerminalDrawerProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    if (!open || !containerName) return;

    let term: Terminal | null = null;
    let socket: WebSocket | null = null;
    let fitAddon: FitAddon | null = null;
    let timer: any;

    const init = () => {
      if (!terminalRef.current) {
        timer = setTimeout(init, 100);
        return;
      }

      term = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: '"Cascadia Code", Menlo, Monaco, "Courier New", monospace',
        theme: {
          background: '#1e1e1e',
        },
      });

      fitAddon = new FitAddon();
      term.loadAddon(fitAddon);
      term.open(terminalRef.current);
      fitAddon.fit();

      xtermRef.current = term;

      // Connect WebSocket
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/api/manage/compute/terminal?container=${containerName}`;
      socket = new WebSocket(wsUrl);
      socketRef.current = socket;

      socket.onopen = () => {
        term?.write('\r\n\x1b[32m[Connected to MiniSky SSH]\x1b[0m\r\n');
      };

      socket.onmessage = async (event) => {
        if (event.data instanceof Blob) {
          const text = await event.data.text();
          term?.write(text);
        } else {
          term?.write(event.data);
        }
      };

      socket.onclose = () => {
        term?.write('\r\n\x1b[31m[Disconnected]\x1b[0m\r\n');
      };

      socket.onerror = () => {
        term?.write('\r\n\x1b[31m[Connection Error]\x1b[0m\r\n');
      };

      term.onData((data: string) => {
        if (socket?.readyState === WebSocket.OPEN) {
          socket.send(data);
        }
      });

      const handleResize = () => fitAddon?.fit();
      window.addEventListener('resize', handleResize);
    };

    init();

    return () => {
      clearTimeout(timer);
      window.removeEventListener('resize', () => fitAddon?.fit());
      socket?.close();
      term?.dispose();
      xtermRef.current = null;
    };
  }, [open, containerName]);

  return (
    <Drawer anchor="bottom" open={open} onClose={onClose}>
      <Box sx={{ height: '60vh', bgcolor: '#1e1e1e', color: '#fff', display: 'flex', flexDirection: 'column' }}>
        <Box sx={{ p: 1, px: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid #333' }}>
          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', opacity: 0.8 }}>
            SSH: {containerName} (root@minisky-vm)
          </Typography>
          <IconButton onClick={onClose} size="small" sx={{ color: '#999' }}>
            <CloseIcon fontSize="small" />
          </IconButton>
        </Box>
        <Box ref={terminalRef} sx={{ flex: 1, p: 1, overflow: 'hidden' }} />
      </Box>
    </Drawer>
  );
}
