// THE CONVERSATION LIST'S BATCH AND ROW ACTIONS, and the conversation's graph
// data, as hooks a look calls instead of reaching the API layer itself
// (design D2). The rules for which rows each may take are in bulk.ts and
// tree.ts.

export {
  useCloseConversations as useCloseBatch,
  useConversationGraph as useConversationGraphData,
  useDeleteConversations as useDeleteBatch,
  useMarkRead,
  useMarkUnread,
  useReopenConversation as useReopen,
  useTopology,
  useVocabulary,
} from '../api/hooks'
export { errorMessage } from './commands'
