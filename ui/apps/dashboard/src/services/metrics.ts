import axios from 'axios';

export interface Pod {
  name: string;
  // Add other properties of a pod if necessary
}
export interface PodsResponse {
  [appName: string]: {
    [appName: string]: Pod[];
  };
}
interface MetricInfo {
    Name: string;
    Type: string;
  }
  
export interface MetricsCategory {
    [key: string]: MetricInfo;
  }
  
  export interface MetricsData {
    [key: string]: {
      [key: string]: MetricInfo;
    };
  }

  export async function getMetrics(appName: string): Promise<MetricsCategory> {
    try {
      const response = await axios.get(`/api/v1/metrics/${appName}`);
      return response.data;
    } catch (error) {
      console.error('Failed to fetch metrics:', error);
      throw error;
    }
  }