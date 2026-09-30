import OpenAI from "openai";
import type { ChatCompletion } from "openai/resources/chat/completions";
import 'dotenv/config';

const openai = new OpenAI({
  apiKey: process.env.NVIDIA_API_KEY,
  baseURL: 'https://integrate.api.nvidia.com/v1',
})

const INGESTION_URL = process.env.TRACELIGHT_INGESTION_URL ?? "http://localhost:8080/trace";
const MODEL = "meta/muse-glimmer-30b";

const sendTrace = (data: object) => {
  fetch(INGESTION_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  }).catch((traceError) => {
    console.error("trace logging failed:", traceError);
  });
};

const trace = async (openFunc: () => Promise<ChatCompletion>) => {
  const trace_id: string = crypto.randomUUID();
  const startTime = Date.now();

  try {
    const responses = await openFunc();
    const duration_ms = Date.now() - startTime;
    console.log("responses", responses);
    sendTrace({
      trace_id,
      provider_response_id: responses.id,
      model: responses.model,
      prompt_tokens: responses.usage?.prompt_tokens ?? null,
      completion_tokens: responses.usage?.completion_tokens ?? null,
      total_tokens: responses.usage?.total_tokens ?? null,
      duration_ms,
      success: true,
      error: null,
      timestamp: new Date().toISOString(),
    });
    console.log("trace sent successfully");
    return responses;
  } catch (error) {
    const duration_ms = Date.now() - startTime;

    sendTrace({
      trace_id,
      provider_response_id: null,
      model: MODEL,
      prompt_tokens: null,
      completion_tokens: null,
      total_tokens: null,
      duration_ms,
      success: false,
      error: {
        name: error instanceof Error ? error.name : "UnknownError",
        message: error instanceof Error ? error.message : String(error),
      },
      timestamp: new Date().toISOString(),
    });

    throw error;
  }
};
var openFunc = async function main() {
  const responses = await openai.chat.completions.create({
    model: "meta/muse-glimmer-30b",
    messages: [{"role":"user","content":"what is calculus?"}],
  });

  return responses;
}
await trace(openFunc);
