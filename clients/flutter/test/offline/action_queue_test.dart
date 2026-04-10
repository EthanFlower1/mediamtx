import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/offline/action_queue.dart';

QueuedAction _action(String id, {ActionType type = ActionType.apiCall}) =>
    QueuedAction(
      id: id,
      actionType: type,
      payload: {'key': 'value'},
      createdAt: DateTime(2026, 1, 1),
    );

void main() {
  late ActionQueue queue;

  setUp(() {
    queue = ActionQueue();
  });

  tearDown(() {
    queue.dispose();
  });

  test('starts empty', () {
    expect(queue.debugState, isEmpty);
  });

  test('enqueue adds action to state', () {
    queue.enqueue(_action('a1'));
    expect(queue.debugState.length, 1);
    expect(queue.debugState.first.id, 'a1');
  });

  test('dequeue returns null when empty', () {
    expect(queue.dequeue(), isNull);
  });

  test('dequeue returns actions in FIFO order', () {
    queue.enqueue(_action('a1'));
    queue.enqueue(_action('a2'));

    expect(queue.dequeue()!.id, 'a1');
    expect(queue.dequeue()!.id, 'a2');
    expect(queue.dequeue(), isNull);
  });

  test('peek returns front without removing', () {
    queue.enqueue(_action('a1'));
    expect(queue.peek()!.id, 'a1');
    expect(queue.debugState.length, 1);
  });

  test('peek returns null when empty', () {
    expect(queue.peek(), isNull);
  });

  test('drain returns all actions and empties queue', () {
    queue.enqueue(_action('a1'));
    queue.enqueue(_action('a2'));
    queue.enqueue(_action('a3'));

    final drained = queue.drain();
    expect(drained.length, 3);
    expect(drained.map((a) => a.id).toList(), ['a1', 'a2', 'a3']);
    expect(queue.debugState, isEmpty);
  });

  test('removeById removes specific action', () {
    queue.enqueue(_action('a1'));
    queue.enqueue(_action('a2'));
    queue.enqueue(_action('a3'));

    queue.removeById('a2');
    expect(queue.debugState.length, 2);
    expect(queue.debugState.map((a) => a.id).toList(), ['a1', 'a3']);
  });

  test('removeById is no-op for missing id', () {
    queue.enqueue(_action('a1'));
    queue.removeById('nonexistent');
    expect(queue.debugState.length, 1);
  });

  test('maxRetries constant is 3', () {
    expect(ActionQueue.maxRetries, 3);
  });

  test('incrementRetry creates copy with retryCount + 1', () {
    final action = _action('a1');
    expect(action.retryCount, 0);

    final retried = action.incrementRetry();
    expect(retried.retryCount, 1);
    expect(retried.id, 'a1');
    expect(retried.actionType, ActionType.apiCall);
  });
}
