import { useState, useEffect } from 'react';

export type Service = {
  id: string;
  name: string;
  label: string;
  status: string;
  port: number | null;
  description: string;
};

export function useServices() {
  const [services, setServices] = useState<Service[]>([]);
  const [settings, setSettings] = useState<any>({});

  const loadData = async () => {
    try {
      const res = await fetch('/api/services');
      if (res.ok) {
        setServices(await res.json());
      }
      const setRes = await fetch('/api/settings');
      if (setRes.ok) {
        setSettings(await setRes.json());
      }
    } catch (e) {
      console.error("error loading UI data", e);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 3000);
    return () => clearInterval(interval);
  }, []);

  const handleStartContainer = async (id: string) => {
    await fetch(`/api/services/${id}/start`, { method: 'POST' });
    loadData();
  };

  const handleStopContainer = async (id: string) => {
    await fetch(`/api/services/${id}/stop`, { method: 'POST' });
    loadData();
  };

  const toggleSetting = async (key: string, currentVal: boolean) => {
    await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ [key]: !currentVal })
    });
    loadData();
  };

  return { services, settings, handleStartContainer, handleStopContainer, toggleSetting };
}
