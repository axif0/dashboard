import { IResponse, karmadaClient } from './base';

export interface MetricInfo {
    help: string;
    type: 'COUNTER' | 'GAUGE' | 'SUMMARY' | 'HISTOGRAM';
}

export interface ClusterMetrics {
    [metricName: string]: MetricInfo;
}

export interface MetricsResponse {
    [clusterName: string]: ClusterMetrics;
}

export interface SyncStatusResponse {
    [componentName: string]: boolean;
}

export async function GetMetricsInfo(componentName: string, type: string): Promise<MetricsResponse> {
    // console.log("componentName", componentName, "type", type);
    const resp = await karmadaClient.get<IResponse<MetricsResponse>>(`/metrics/${componentName}?type=${type}`);
    // console.log("resp.data", resp.data);
    return resp.data as unknown as MetricsResponse;  // Convert to unknown first, then assert as MetricsResponse
}

export interface MetricValue {
    value: string;
    measure: string;
    labels: {
        [key: string]: string;
    };
}

export interface MetricDetails {
    name: string;
    values: MetricValue[];
}

export interface MetricDetailsResponse {
    details: {
        [timestamp: string]: MetricDetails;
    };
}

export async function GetMetricsDetails(componentName: string, podsName: string, metricName: string): Promise<MetricDetailsResponse> {
    console.log("componentName", componentName, "podsName", podsName, "metricName", metricName);
    const resp = await karmadaClient.get<MetricDetailsResponse>(`/metrics/${componentName}/${podsName}?type=details&mname=${metricName}`);
    console.log("resp.data GetMetricsDetails", resp.data);
    return resp.data;
}

interface SyncSettingResponse {
    message: string;
}

export async function UpdateSyncSetting(componentName: string, action: 'sync_on' | 'sync_off'): Promise<string> {
    const resp = await karmadaClient.get<IResponse<SyncSettingResponse>>(`/metrics/${componentName}?type=${action}`);
    return resp.data.message;
}

export async function GetSyncStatus(): Promise<SyncStatusResponse> {
    const resp = await karmadaClient.get<SyncStatusResponse>('/metrics?type=sync_status');
    return resp.data;
}