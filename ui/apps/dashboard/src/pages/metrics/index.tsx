import React, { useState, useEffect } from 'react';
import Panel from '@/components/panel';
import { Menu, Button, Spin, Table } from 'antd';
import appNames from './demomatrics.json';
import { getMetrics, MetricsData } from '@/services/metrics';

const Metrics: React.FC = () => {
  const [selectedApp, setSelectedApp] = useState<string | null>(null);
  const [metricsData, setMetricsData] = useState<MetricsData | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (selectedApp) {
      fetchMetrics(selectedApp);
    }
  }, [selectedApp]);

  const fetchMetrics = async (appName: string) => {
    setLoading(true);
    try {
      const data = await getMetrics(appName);
      setMetricsData({ [appName]: data });
    } catch (error) {
      console.error('Failed to fetch metrics:', error);
    }
    setLoading(false);
  };

  const handleAppClick = (key: string) => {
    setSelectedApp(key);
  };

  const menuItems = appNames.map((appName) => ({
    key: appName,
    label: (
      <Button
        onClick={() => handleAppClick(appName)}
        aria-haspopup="true"
        aria-expanded={selectedApp === appName}
      >
        {appName}
      </Button>
    ),
  }));

  const renderMetricsTable = () => {
    if (!metricsData) return null;

    const columns = [
      { title: 'Metric Name', dataIndex: 'name', key: 'name' },
      { title: 'Type', dataIndex: 'type', key: 'type' },
    ];

    const data = Object.entries(metricsData[selectedApp!]).map(([key, value]) => ({
      key,
      name: value.Name,
      type: value.Type,
    }));

    return <Table columns={columns} dataSource={data} />;
  };

  return (
    <Panel>
      <div style={{ display: 'flex' }}>
        <Menu
          mode="inline"
          style={{
            padding: '10px',
            width: '240px',
            height: '100%',
            maxHeight: '550px',
            overflowY: 'auto',
            borderRight: 0,
          }}
          items={menuItems}
        />
        <div style={{ marginLeft: '20px', flexGrow: 1 }}>
          <h4>Metrics Information:</h4>
          {selectedApp ? (
            <div style={{ marginTop: '10px', padding: '10px', border: '1px solid #ccc', borderRadius: '4px' }}>
              <h5>Selected App: {selectedApp}</h5>
              {loading ? (
                <Spin />
              ) : (
                renderMetricsTable()
              )}
            </div>
          ) : (
            <p>Please select an app from the menu to view its metrics.</p>
          )}
        </div>
      </div>
    </Panel>
  );
};

export default Metrics;