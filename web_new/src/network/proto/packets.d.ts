import * as $protobuf from "protobufjs";
import Long = require("long");
/** Namespace proto. */
export namespace proto {

    /** Properties of a Position. */
    interface IPosition {

        /** Position x */
        x?: (number|null);

        /** Position y */
        y?: (number|null);

        /** Position heading */
        heading?: (number|null);
    }

    /** Represents a Position. */
    class Position implements IPosition {

        /**
         * Constructs a new Position.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IPosition);

        /** Position x. */
        public x: number;

        /** Position y. */
        public y: number;

        /** Position heading. */
        public heading: number;

        /**
         * Creates a new Position instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Position instance
         */
        public static create(properties?: proto.IPosition): proto.Position;

        /**
         * Encodes the specified Position message. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @param message Position message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IPosition, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Position message, length delimited. Does not implicitly {@link proto.Position.verify|verify} messages.
         * @param message Position message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IPosition, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a Position message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.Position;

        /**
         * Decodes a Position message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Position
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.Position;

        /**
         * Verifies a Position message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a Position message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Position
         */
        public static fromObject(object: { [k: string]: any }): proto.Position;

        /**
         * Creates a plain object from a Position message. Also converts values to other types if specified.
         * @param message Position
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.Position, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Position to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Position
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a Vector2. */
    interface IVector2 {

        /** Vector2 x */
        x?: (number|null);

        /** Vector2 y */
        y?: (number|null);
    }

    /** Represents a Vector2. */
    class Vector2 implements IVector2 {

        /**
         * Constructs a new Vector2.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IVector2);

        /** Vector2 x. */
        public x: number;

        /** Vector2 y. */
        public y: number;

        /**
         * Creates a new Vector2 instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Vector2 instance
         */
        public static create(properties?: proto.IVector2): proto.Vector2;

        /**
         * Encodes the specified Vector2 message. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @param message Vector2 message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IVector2, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Vector2 message, length delimited. Does not implicitly {@link proto.Vector2.verify|verify} messages.
         * @param message Vector2 message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IVector2, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a Vector2 message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.Vector2;

        /**
         * Decodes a Vector2 message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Vector2
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.Vector2;

        /**
         * Verifies a Vector2 message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a Vector2 message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Vector2
         */
        public static fromObject(object: { [k: string]: any }): proto.Vector2;

        /**
         * Creates a plain object from a Vector2 message. Also converts values to other types if specified.
         * @param message Vector2
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.Vector2, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Vector2 to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Vector2
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a AABB. */
    interface IAABB {

        /** AABB minX */
        minX?: (number|null);

        /** AABB minY */
        minY?: (number|null);

        /** AABB maxX */
        maxX?: (number|null);

        /** AABB maxY */
        maxY?: (number|null);
    }

    /** Represents a AABB. */
    class AABB implements IAABB {

        /**
         * Constructs a new AABB.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IAABB);

        /** AABB minX. */
        public minX: number;

        /** AABB minY. */
        public minY: number;

        /** AABB maxX. */
        public maxX: number;

        /** AABB maxY. */
        public maxY: number;

        /**
         * Creates a new AABB instance using the specified properties.
         * @param [properties] Properties to set
         * @returns AABB instance
         */
        public static create(properties?: proto.IAABB): proto.AABB;

        /**
         * Encodes the specified AABB message. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @param message AABB message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IAABB, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified AABB message, length delimited. Does not implicitly {@link proto.AABB.verify|verify} messages.
         * @param message AABB message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IAABB, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a AABB message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.AABB;

        /**
         * Decodes a AABB message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns AABB
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.AABB;

        /**
         * Verifies a AABB message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a AABB message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns AABB
         */
        public static fromObject(object: { [k: string]: any }): proto.AABB;

        /**
         * Creates a plain object from a AABB message. Also converts values to other types if specified.
         * @param message AABB
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.AABB, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this AABB to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for AABB
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a Timestamp. */
    interface ITimestamp {

        /** Timestamp millis */
        millis?: (number|Long|null);
    }

    /** Represents a Timestamp. */
    class Timestamp implements ITimestamp {

        /**
         * Constructs a new Timestamp.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.ITimestamp);

        /** Timestamp millis. */
        public millis: (number|Long);

        /**
         * Creates a new Timestamp instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Timestamp instance
         */
        public static create(properties?: proto.ITimestamp): proto.Timestamp;

        /**
         * Encodes the specified Timestamp message. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @param message Timestamp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.ITimestamp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Timestamp message, length delimited. Does not implicitly {@link proto.Timestamp.verify|verify} messages.
         * @param message Timestamp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.ITimestamp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a Timestamp message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.Timestamp;

        /**
         * Decodes a Timestamp message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Timestamp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.Timestamp;

        /**
         * Verifies a Timestamp message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a Timestamp message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Timestamp
         */
        public static fromObject(object: { [k: string]: any }): proto.Timestamp;

        /**
         * Creates a plain object from a Timestamp message. Also converts values to other types if specified.
         * @param message Timestamp
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.Timestamp, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Timestamp to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Timestamp
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** MovementMode enum. */
    enum MovementMode {
        MOVE_MODE_WALK = 0,
        MOVE_MODE_RUN = 1,
        MOVE_MODE_FAST_RUN = 2,
        MOVE_MODE_SWIM = 3
    }

    /** EquipSlot enum. */
    enum EquipSlot {
        EQUIP_SLOT_NONE = 0,
        EQUIP_SLOT_HEAD = 1,
        EQUIP_SLOT_CHEST = 2,
        EQUIP_SLOT_LEGS = 3,
        EQUIP_SLOT_FEET = 4,
        EQUIP_SLOT_HANDS = 5,
        EQUIP_SLOT_LEFT_HAND = 6,
        EQUIP_SLOT_RIGHT_HAND = 7,
        EQUIP_SLOT_BACK = 8,
        EQUIP_SLOT_NECK = 9,
        EQUIP_SLOT_RING_1 = 10,
        EQUIP_SLOT_RING_2 = 11
    }

    /** ExpType enum. */
    enum ExpType {
        EXP_TYPE_NATURE = 0,
        EXP_TYPE_INDUSTRY = 1,
        EXP_TYPE_COMBAT = 2
    }

    /** WeatherType enum. */
    enum WeatherType {
        WEATHER_TYPE_CLEAR = 0,
        WEATHER_TYPE_RAIN = 1,
        WEATHER_TYPE_FOG = 2,
        WEATHER_TYPE_STORM = 3,
        WEATHER_TYPE_SNOW = 4
    }

    /** InventoryKind enum. */
    enum InventoryKind {
        INVENTORY_KIND_GRID = 0,
        INVENTORY_KIND_HAND = 1,
        INVENTORY_KIND_EQUIPMENT = 2,
        INVENTORY_KIND_DROPPED_ITEM = 3
    }

    /** ErrorCode enum. */
    enum ErrorCode {
        ERROR_CODE_NONE = 0,
        ERROR_CODE_INVALID_REQUEST = 1,
        ERROR_CODE_NOT_AUTHENTICATED = 2,
        ERROR_CODE_ENTITY_NOT_FOUND = 3,
        ERROR_CODE_OUT_OF_RANGE = 4,
        ERROR_CODE_INSUFFICIENT_RESOURCES = 5,
        ERROR_CODE_INVENTORY_FULL = 6,
        ERROR_CODE_CANNOT_INTERACT = 7,
        ERROR_CODE_COOLDOWN_ACTIVE = 8,
        ERROR_CODE_INSUFFICIENT_STAMINA = 9,
        ERROR_CODE_TARGET_INVALID = 10,
        ERROR_CODE_PATH_BLOCKED = 11,
        ERROR_CODE_TIMEOUT_EXCEEDED = 12,
        ERROR_CODE_BUILDING_INCOMPLETE = 13,
        ERROR_CODE_RECIPE_UNKNOWN = 14,
        ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED = 15,
        ERROR_CODE_INTERNAL_ERROR = 16
    }

    /** WarningCode enum. */
    enum WarningCode {
        WARN_INPUT_QUEUE_OVERFLOW = 0
    }

    /** Properties of an InventoryRef. */
    interface IInventoryRef {

        /** InventoryRef kind */
        kind?: (proto.InventoryKind|null);

        /** InventoryRef ownerEntityId */
        ownerEntityId?: (number|Long|null);

        /** InventoryRef inventoryKey */
        inventoryKey?: (number|null);
    }

    /** Represents an InventoryRef. */
    class InventoryRef implements IInventoryRef {

        /**
         * Constructs a new InventoryRef.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryRef);

        /** InventoryRef kind. */
        public kind: proto.InventoryKind;

        /** InventoryRef ownerEntityId. */
        public ownerEntityId: (number|Long);

        /** InventoryRef inventoryKey. */
        public inventoryKey: number;

        /**
         * Creates a new InventoryRef instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryRef instance
         */
        public static create(properties?: proto.IInventoryRef): proto.InventoryRef;

        /**
         * Encodes the specified InventoryRef message. Does not implicitly {@link proto.InventoryRef.verify|verify} messages.
         * @param message InventoryRef message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryRef, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryRef message, length delimited. Does not implicitly {@link proto.InventoryRef.verify|verify} messages.
         * @param message InventoryRef message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryRef, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryRef message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryRef
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryRef;

        /**
         * Decodes an InventoryRef message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryRef
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryRef;

        /**
         * Verifies an InventoryRef message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryRef message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryRef
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryRef;

        /**
         * Creates a plain object from an InventoryRef message. Also converts values to other types if specified.
         * @param message InventoryRef
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryRef, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryRef to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryRef
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an ItemInstance. */
    interface IItemInstance {

        /** ItemInstance itemId */
        itemId?: (number|Long|null);

        /** ItemInstance typeId */
        typeId?: (number|null);

        /** ItemInstance resource */
        resource?: (string|null);

        /** ItemInstance quality */
        quality?: (number|null);

        /** ItemInstance quantity */
        quantity?: (number|null);

        /** ItemInstance w */
        w?: (number|null);

        /** ItemInstance h */
        h?: (number|null);
    }

    /** Represents an ItemInstance. */
    class ItemInstance implements IItemInstance {

        /**
         * Constructs a new ItemInstance.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IItemInstance);

        /** ItemInstance itemId. */
        public itemId: (number|Long);

        /** ItemInstance typeId. */
        public typeId: number;

        /** ItemInstance resource. */
        public resource: string;

        /** ItemInstance quality. */
        public quality: number;

        /** ItemInstance quantity. */
        public quantity: number;

        /** ItemInstance w. */
        public w: number;

        /** ItemInstance h. */
        public h: number;

        /**
         * Creates a new ItemInstance instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ItemInstance instance
         */
        public static create(properties?: proto.IItemInstance): proto.ItemInstance;

        /**
         * Encodes the specified ItemInstance message. Does not implicitly {@link proto.ItemInstance.verify|verify} messages.
         * @param message ItemInstance message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IItemInstance, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ItemInstance message, length delimited. Does not implicitly {@link proto.ItemInstance.verify|verify} messages.
         * @param message ItemInstance message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IItemInstance, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an ItemInstance message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ItemInstance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.ItemInstance;

        /**
         * Decodes an ItemInstance message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ItemInstance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.ItemInstance;

        /**
         * Verifies an ItemInstance message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an ItemInstance message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ItemInstance
         */
        public static fromObject(object: { [k: string]: any }): proto.ItemInstance;

        /**
         * Creates a plain object from an ItemInstance message. Also converts values to other types if specified.
         * @param message ItemInstance
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.ItemInstance, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ItemInstance to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ItemInstance
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a GridItem. */
    interface IGridItem {

        /** GridItem x */
        x?: (number|null);

        /** GridItem y */
        y?: (number|null);

        /** GridItem item */
        item?: (proto.IItemInstance|null);
    }

    /** Represents a GridItem. */
    class GridItem implements IGridItem {

        /**
         * Constructs a new GridItem.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IGridItem);

        /** GridItem x. */
        public x: number;

        /** GridItem y. */
        public y: number;

        /** GridItem item. */
        public item?: (proto.IItemInstance|null);

        /**
         * Creates a new GridItem instance using the specified properties.
         * @param [properties] Properties to set
         * @returns GridItem instance
         */
        public static create(properties?: proto.IGridItem): proto.GridItem;

        /**
         * Encodes the specified GridItem message. Does not implicitly {@link proto.GridItem.verify|verify} messages.
         * @param message GridItem message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IGridItem, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified GridItem message, length delimited. Does not implicitly {@link proto.GridItem.verify|verify} messages.
         * @param message GridItem message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IGridItem, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a GridItem message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns GridItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.GridItem;

        /**
         * Decodes a GridItem message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns GridItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.GridItem;

        /**
         * Verifies a GridItem message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a GridItem message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns GridItem
         */
        public static fromObject(object: { [k: string]: any }): proto.GridItem;

        /**
         * Creates a plain object from a GridItem message. Also converts values to other types if specified.
         * @param message GridItem
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.GridItem, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this GridItem to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for GridItem
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryGridState. */
    interface IInventoryGridState {

        /** InventoryGridState width */
        width?: (number|null);

        /** InventoryGridState height */
        height?: (number|null);

        /** InventoryGridState items */
        items?: (proto.IGridItem[]|null);
    }

    /** Represents an InventoryGridState. */
    class InventoryGridState implements IInventoryGridState {

        /**
         * Constructs a new InventoryGridState.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryGridState);

        /** InventoryGridState width. */
        public width: number;

        /** InventoryGridState height. */
        public height: number;

        /** InventoryGridState items. */
        public items: proto.IGridItem[];

        /**
         * Creates a new InventoryGridState instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryGridState instance
         */
        public static create(properties?: proto.IInventoryGridState): proto.InventoryGridState;

        /**
         * Encodes the specified InventoryGridState message. Does not implicitly {@link proto.InventoryGridState.verify|verify} messages.
         * @param message InventoryGridState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryGridState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryGridState message, length delimited. Does not implicitly {@link proto.InventoryGridState.verify|verify} messages.
         * @param message InventoryGridState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryGridState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryGridState message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryGridState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryGridState;

        /**
         * Decodes an InventoryGridState message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryGridState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryGridState;

        /**
         * Verifies an InventoryGridState message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryGridState message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryGridState
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryGridState;

        /**
         * Creates a plain object from an InventoryGridState message. Also converts values to other types if specified.
         * @param message InventoryGridState
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryGridState, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryGridState to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryGridState
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an EquipmentItem. */
    interface IEquipmentItem {

        /** EquipmentItem slot */
        slot?: (proto.EquipSlot|null);

        /** EquipmentItem item */
        item?: (proto.IItemInstance|null);
    }

    /** Represents an EquipmentItem. */
    class EquipmentItem implements IEquipmentItem {

        /**
         * Constructs a new EquipmentItem.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IEquipmentItem);

        /** EquipmentItem slot. */
        public slot: proto.EquipSlot;

        /** EquipmentItem item. */
        public item?: (proto.IItemInstance|null);

        /**
         * Creates a new EquipmentItem instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EquipmentItem instance
         */
        public static create(properties?: proto.IEquipmentItem): proto.EquipmentItem;

        /**
         * Encodes the specified EquipmentItem message. Does not implicitly {@link proto.EquipmentItem.verify|verify} messages.
         * @param message EquipmentItem message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IEquipmentItem, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EquipmentItem message, length delimited. Does not implicitly {@link proto.EquipmentItem.verify|verify} messages.
         * @param message EquipmentItem message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IEquipmentItem, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EquipmentItem message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EquipmentItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.EquipmentItem;

        /**
         * Decodes an EquipmentItem message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EquipmentItem
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.EquipmentItem;

        /**
         * Verifies an EquipmentItem message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EquipmentItem message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EquipmentItem
         */
        public static fromObject(object: { [k: string]: any }): proto.EquipmentItem;

        /**
         * Creates a plain object from an EquipmentItem message. Also converts values to other types if specified.
         * @param message EquipmentItem
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.EquipmentItem, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EquipmentItem to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EquipmentItem
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryEquipmentState. */
    interface IInventoryEquipmentState {

        /** InventoryEquipmentState items */
        items?: (proto.IEquipmentItem[]|null);
    }

    /** Represents an InventoryEquipmentState. */
    class InventoryEquipmentState implements IInventoryEquipmentState {

        /**
         * Constructs a new InventoryEquipmentState.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryEquipmentState);

        /** InventoryEquipmentState items. */
        public items: proto.IEquipmentItem[];

        /**
         * Creates a new InventoryEquipmentState instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryEquipmentState instance
         */
        public static create(properties?: proto.IInventoryEquipmentState): proto.InventoryEquipmentState;

        /**
         * Encodes the specified InventoryEquipmentState message. Does not implicitly {@link proto.InventoryEquipmentState.verify|verify} messages.
         * @param message InventoryEquipmentState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryEquipmentState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryEquipmentState message, length delimited. Does not implicitly {@link proto.InventoryEquipmentState.verify|verify} messages.
         * @param message InventoryEquipmentState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryEquipmentState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryEquipmentState message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryEquipmentState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryEquipmentState;

        /**
         * Decodes an InventoryEquipmentState message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryEquipmentState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryEquipmentState;

        /**
         * Verifies an InventoryEquipmentState message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryEquipmentState message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryEquipmentState
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryEquipmentState;

        /**
         * Creates a plain object from an InventoryEquipmentState message. Also converts values to other types if specified.
         * @param message InventoryEquipmentState
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryEquipmentState, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryEquipmentState to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryEquipmentState
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryHandState. */
    interface IInventoryHandState {

        /** InventoryHandState item */
        item?: (proto.IItemInstance|null);
    }

    /** Represents an InventoryHandState. */
    class InventoryHandState implements IInventoryHandState {

        /**
         * Constructs a new InventoryHandState.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryHandState);

        /** InventoryHandState item. */
        public item?: (proto.IItemInstance|null);

        /**
         * Creates a new InventoryHandState instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryHandState instance
         */
        public static create(properties?: proto.IInventoryHandState): proto.InventoryHandState;

        /**
         * Encodes the specified InventoryHandState message. Does not implicitly {@link proto.InventoryHandState.verify|verify} messages.
         * @param message InventoryHandState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryHandState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryHandState message, length delimited. Does not implicitly {@link proto.InventoryHandState.verify|verify} messages.
         * @param message InventoryHandState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryHandState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryHandState message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryHandState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryHandState;

        /**
         * Decodes an InventoryHandState message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryHandState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryHandState;

        /**
         * Verifies an InventoryHandState message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryHandState message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryHandState
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryHandState;

        /**
         * Creates a plain object from an InventoryHandState message. Also converts values to other types if specified.
         * @param message InventoryHandState
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryHandState, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryHandState to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryHandState
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryState. */
    interface IInventoryState {

        /** InventoryState ref */
        ref?: (proto.IInventoryRef|null);

        /** InventoryState revision */
        revision?: (number|Long|null);

        /** InventoryState grid */
        grid?: (proto.IInventoryGridState|null);

        /** InventoryState equipment */
        equipment?: (proto.IInventoryEquipmentState|null);

        /** InventoryState hand */
        hand?: (proto.IInventoryHandState|null);
    }

    /** Represents an InventoryState. */
    class InventoryState implements IInventoryState {

        /**
         * Constructs a new InventoryState.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryState);

        /** InventoryState ref. */
        public ref?: (proto.IInventoryRef|null);

        /** InventoryState revision. */
        public revision: (number|Long);

        /** InventoryState grid. */
        public grid?: (proto.IInventoryGridState|null);

        /** InventoryState equipment. */
        public equipment?: (proto.IInventoryEquipmentState|null);

        /** InventoryState hand. */
        public hand?: (proto.IInventoryHandState|null);

        /** InventoryState state. */
        public state?: ("grid"|"equipment"|"hand");

        /**
         * Creates a new InventoryState instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryState instance
         */
        public static create(properties?: proto.IInventoryState): proto.InventoryState;

        /**
         * Encodes the specified InventoryState message. Does not implicitly {@link proto.InventoryState.verify|verify} messages.
         * @param message InventoryState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryState message, length delimited. Does not implicitly {@link proto.InventoryState.verify|verify} messages.
         * @param message InventoryState message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryState, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryState message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryState;

        /**
         * Decodes an InventoryState message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryState
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryState;

        /**
         * Verifies an InventoryState message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryState message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryState
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryState;

        /**
         * Creates a plain object from an InventoryState message. Also converts values to other types if specified.
         * @param message InventoryState
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryState, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryState to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryState
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryExpected. */
    interface IInventoryExpected {

        /** InventoryExpected ref */
        ref?: (proto.IInventoryRef|null);

        /** InventoryExpected expectedRevision */
        expectedRevision?: (number|Long|null);
    }

    /** Represents an InventoryExpected. */
    class InventoryExpected implements IInventoryExpected {

        /**
         * Constructs a new InventoryExpected.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryExpected);

        /** InventoryExpected ref. */
        public ref?: (proto.IInventoryRef|null);

        /** InventoryExpected expectedRevision. */
        public expectedRevision: (number|Long);

        /**
         * Creates a new InventoryExpected instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryExpected instance
         */
        public static create(properties?: proto.IInventoryExpected): proto.InventoryExpected;

        /**
         * Encodes the specified InventoryExpected message. Does not implicitly {@link proto.InventoryExpected.verify|verify} messages.
         * @param message InventoryExpected message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryExpected, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryExpected message, length delimited. Does not implicitly {@link proto.InventoryExpected.verify|verify} messages.
         * @param message InventoryExpected message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryExpected, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryExpected message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryExpected
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryExpected;

        /**
         * Decodes an InventoryExpected message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryExpected
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryExpected;

        /**
         * Verifies an InventoryExpected message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryExpected message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryExpected
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryExpected;

        /**
         * Creates a plain object from an InventoryExpected message. Also converts values to other types if specified.
         * @param message InventoryExpected
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryExpected, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryExpected to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryExpected
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a GridPos. */
    interface IGridPos {

        /** GridPos x */
        x?: (number|null);

        /** GridPos y */
        y?: (number|null);
    }

    /** Represents a GridPos. */
    class GridPos implements IGridPos {

        /**
         * Constructs a new GridPos.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IGridPos);

        /** GridPos x. */
        public x: number;

        /** GridPos y. */
        public y: number;

        /**
         * Creates a new GridPos instance using the specified properties.
         * @param [properties] Properties to set
         * @returns GridPos instance
         */
        public static create(properties?: proto.IGridPos): proto.GridPos;

        /**
         * Encodes the specified GridPos message. Does not implicitly {@link proto.GridPos.verify|verify} messages.
         * @param message GridPos message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IGridPos, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified GridPos message, length delimited. Does not implicitly {@link proto.GridPos.verify|verify} messages.
         * @param message GridPos message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IGridPos, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a GridPos message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns GridPos
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.GridPos;

        /**
         * Decodes a GridPos message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns GridPos
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.GridPos;

        /**
         * Verifies a GridPos message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a GridPos message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns GridPos
         */
        public static fromObject(object: { [k: string]: any }): proto.GridPos;

        /**
         * Creates a plain object from a GridPos message. Also converts values to other types if specified.
         * @param message GridPos
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.GridPos, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this GridPos to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for GridPos
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryMoveSpec. */
    interface IInventoryMoveSpec {

        /** InventoryMoveSpec src */
        src?: (proto.IInventoryRef|null);

        /** InventoryMoveSpec dst */
        dst?: (proto.IInventoryRef|null);

        /** InventoryMoveSpec itemId */
        itemId?: (number|Long|null);

        /** InventoryMoveSpec dstPos */
        dstPos?: (proto.IGridPos|null);

        /** InventoryMoveSpec dstEquipSlot */
        dstEquipSlot?: (proto.EquipSlot|null);

        /** InventoryMoveSpec quantity */
        quantity?: (number|null);

        /** InventoryMoveSpec allowSwapOrMerge */
        allowSwapOrMerge?: (boolean|null);
    }

    /** Represents an InventoryMoveSpec. */
    class InventoryMoveSpec implements IInventoryMoveSpec {

        /**
         * Constructs a new InventoryMoveSpec.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryMoveSpec);

        /** InventoryMoveSpec src. */
        public src?: (proto.IInventoryRef|null);

        /** InventoryMoveSpec dst. */
        public dst?: (proto.IInventoryRef|null);

        /** InventoryMoveSpec itemId. */
        public itemId: (number|Long);

        /** InventoryMoveSpec dstPos. */
        public dstPos?: (proto.IGridPos|null);

        /** InventoryMoveSpec dstEquipSlot. */
        public dstEquipSlot?: (proto.EquipSlot|null);

        /** InventoryMoveSpec quantity. */
        public quantity?: (number|null);

        /** InventoryMoveSpec allowSwapOrMerge. */
        public allowSwapOrMerge: boolean;

        /**
         * Creates a new InventoryMoveSpec instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryMoveSpec instance
         */
        public static create(properties?: proto.IInventoryMoveSpec): proto.InventoryMoveSpec;

        /**
         * Encodes the specified InventoryMoveSpec message. Does not implicitly {@link proto.InventoryMoveSpec.verify|verify} messages.
         * @param message InventoryMoveSpec message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryMoveSpec, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryMoveSpec message, length delimited. Does not implicitly {@link proto.InventoryMoveSpec.verify|verify} messages.
         * @param message InventoryMoveSpec message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryMoveSpec, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryMoveSpec message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryMoveSpec
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryMoveSpec;

        /**
         * Decodes an InventoryMoveSpec message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryMoveSpec
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryMoveSpec;

        /**
         * Verifies an InventoryMoveSpec message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryMoveSpec message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryMoveSpec
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryMoveSpec;

        /**
         * Creates a plain object from an InventoryMoveSpec message. Also converts values to other types if specified.
         * @param message InventoryMoveSpec
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryMoveSpec, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryMoveSpec to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryMoveSpec
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an InventoryOp. */
    interface IInventoryOp {

        /** InventoryOp opId */
        opId?: (number|Long|null);

        /** InventoryOp expected */
        expected?: (proto.IInventoryExpected[]|null);

        /** InventoryOp move */
        move?: (proto.IInventoryMoveSpec|null);

        /** InventoryOp dropToWorld */
        dropToWorld?: (proto.IInventoryMoveSpec|null);

        /** InventoryOp pickupFromWorld */
        pickupFromWorld?: (proto.IInventoryMoveSpec|null);
    }

    /** Represents an InventoryOp. */
    class InventoryOp implements IInventoryOp {

        /**
         * Constructs a new InventoryOp.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInventoryOp);

        /** InventoryOp opId. */
        public opId: (number|Long);

        /** InventoryOp expected. */
        public expected: proto.IInventoryExpected[];

        /** InventoryOp move. */
        public move?: (proto.IInventoryMoveSpec|null);

        /** InventoryOp dropToWorld. */
        public dropToWorld?: (proto.IInventoryMoveSpec|null);

        /** InventoryOp pickupFromWorld. */
        public pickupFromWorld?: (proto.IInventoryMoveSpec|null);

        /** InventoryOp kind. */
        public kind?: ("move"|"dropToWorld"|"pickupFromWorld");

        /**
         * Creates a new InventoryOp instance using the specified properties.
         * @param [properties] Properties to set
         * @returns InventoryOp instance
         */
        public static create(properties?: proto.IInventoryOp): proto.InventoryOp;

        /**
         * Encodes the specified InventoryOp message. Does not implicitly {@link proto.InventoryOp.verify|verify} messages.
         * @param message InventoryOp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInventoryOp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified InventoryOp message, length delimited. Does not implicitly {@link proto.InventoryOp.verify|verify} messages.
         * @param message InventoryOp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInventoryOp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an InventoryOp message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.InventoryOp;

        /**
         * Decodes an InventoryOp message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.InventoryOp;

        /**
         * Verifies an InventoryOp message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an InventoryOp message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns InventoryOp
         */
        public static fromObject(object: { [k: string]: any }): proto.InventoryOp;

        /**
         * Creates a plain object from an InventoryOp message. Also converts values to other types if specified.
         * @param message InventoryOp
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.InventoryOp, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this InventoryOp to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for InventoryOp
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a C2S_InventoryOp. */
    interface IC2S_InventoryOp {

        /** C2S_InventoryOp op */
        op?: (proto.IInventoryOp|null);
    }

    /** Represents a C2S_InventoryOp. */
    class C2S_InventoryOp implements IC2S_InventoryOp {

        /**
         * Constructs a new C2S_InventoryOp.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IC2S_InventoryOp);

        /** C2S_InventoryOp op. */
        public op?: (proto.IInventoryOp|null);

        /**
         * Creates a new C2S_InventoryOp instance using the specified properties.
         * @param [properties] Properties to set
         * @returns C2S_InventoryOp instance
         */
        public static create(properties?: proto.IC2S_InventoryOp): proto.C2S_InventoryOp;

        /**
         * Encodes the specified C2S_InventoryOp message. Does not implicitly {@link proto.C2S_InventoryOp.verify|verify} messages.
         * @param message C2S_InventoryOp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IC2S_InventoryOp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified C2S_InventoryOp message, length delimited. Does not implicitly {@link proto.C2S_InventoryOp.verify|verify} messages.
         * @param message C2S_InventoryOp message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IC2S_InventoryOp, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a C2S_InventoryOp message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns C2S_InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.C2S_InventoryOp;

        /**
         * Decodes a C2S_InventoryOp message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns C2S_InventoryOp
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.C2S_InventoryOp;

        /**
         * Verifies a C2S_InventoryOp message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a C2S_InventoryOp message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns C2S_InventoryOp
         */
        public static fromObject(object: { [k: string]: any }): proto.C2S_InventoryOp;

        /**
         * Creates a plain object from a C2S_InventoryOp message. Also converts values to other types if specified.
         * @param message C2S_InventoryOp
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.C2S_InventoryOp, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this C2S_InventoryOp to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for C2S_InventoryOp
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an EntityMovement. */
    interface IEntityMovement {

        /** EntityMovement position */
        position?: (proto.IPosition|null);

        /** EntityMovement velocity */
        velocity?: (proto.IVector2|null);

        /** EntityMovement moveMode */
        moveMode?: (proto.MovementMode|null);

        /** EntityMovement targetPosition */
        targetPosition?: (proto.IVector2|null);

        /** EntityMovement isMoving */
        isMoving?: (boolean|null);
    }

    /** Represents an EntityMovement. */
    class EntityMovement implements IEntityMovement {

        /**
         * Constructs a new EntityMovement.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IEntityMovement);

        /** EntityMovement position. */
        public position?: (proto.IPosition|null);

        /** EntityMovement velocity. */
        public velocity?: (proto.IVector2|null);

        /** EntityMovement moveMode. */
        public moveMode: proto.MovementMode;

        /** EntityMovement targetPosition. */
        public targetPosition?: (proto.IVector2|null);

        /** EntityMovement isMoving. */
        public isMoving: boolean;

        /**
         * Creates a new EntityMovement instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EntityMovement instance
         */
        public static create(properties?: proto.IEntityMovement): proto.EntityMovement;

        /**
         * Encodes the specified EntityMovement message. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @param message EntityMovement message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IEntityMovement, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EntityMovement message, length delimited. Does not implicitly {@link proto.EntityMovement.verify|verify} messages.
         * @param message EntityMovement message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IEntityMovement, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EntityMovement message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.EntityMovement;

        /**
         * Decodes an EntityMovement message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EntityMovement
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.EntityMovement;

        /**
         * Verifies an EntityMovement message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EntityMovement message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EntityMovement
         */
        public static fromObject(object: { [k: string]: any }): proto.EntityMovement;

        /**
         * Creates a plain object from an EntityMovement message. Also converts values to other types if specified.
         * @param message EntityMovement
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.EntityMovement, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EntityMovement to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EntityMovement
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an EntityPosition. */
    interface IEntityPosition {

        /** EntityPosition position */
        position?: (proto.IPosition|null);

        /** EntityPosition size */
        size?: (proto.IVector2|null);
    }

    /** Represents an EntityPosition. */
    class EntityPosition implements IEntityPosition {

        /**
         * Constructs a new EntityPosition.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IEntityPosition);

        /** EntityPosition position. */
        public position?: (proto.IPosition|null);

        /** EntityPosition size. */
        public size?: (proto.IVector2|null);

        /**
         * Creates a new EntityPosition instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EntityPosition instance
         */
        public static create(properties?: proto.IEntityPosition): proto.EntityPosition;

        /**
         * Encodes the specified EntityPosition message. Does not implicitly {@link proto.EntityPosition.verify|verify} messages.
         * @param message EntityPosition message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IEntityPosition, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EntityPosition message, length delimited. Does not implicitly {@link proto.EntityPosition.verify|verify} messages.
         * @param message EntityPosition message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IEntityPosition, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EntityPosition message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EntityPosition
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.EntityPosition;

        /**
         * Decodes an EntityPosition message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EntityPosition
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.EntityPosition;

        /**
         * Verifies an EntityPosition message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EntityPosition message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EntityPosition
         */
        public static fromObject(object: { [k: string]: any }): proto.EntityPosition;

        /**
         * Creates a plain object from an EntityPosition message. Also converts values to other types if specified.
         * @param message EntityPosition
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.EntityPosition, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EntityPosition to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EntityPosition
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an EntityAppearance. */
    interface IEntityAppearance {

        /** EntityAppearance resource */
        resource?: (string|null);

        /** EntityAppearance name */
        name?: (string|null);
    }

    /** Represents an EntityAppearance. */
    class EntityAppearance implements IEntityAppearance {

        /**
         * Constructs a new EntityAppearance.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IEntityAppearance);

        /** EntityAppearance resource. */
        public resource: string;

        /** EntityAppearance name. */
        public name: string;

        /**
         * Creates a new EntityAppearance instance using the specified properties.
         * @param [properties] Properties to set
         * @returns EntityAppearance instance
         */
        public static create(properties?: proto.IEntityAppearance): proto.EntityAppearance;

        /**
         * Encodes the specified EntityAppearance message. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @param message EntityAppearance message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IEntityAppearance, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified EntityAppearance message, length delimited. Does not implicitly {@link proto.EntityAppearance.verify|verify} messages.
         * @param message EntityAppearance message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IEntityAppearance, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.EntityAppearance;

        /**
         * Decodes an EntityAppearance message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns EntityAppearance
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.EntityAppearance;

        /**
         * Verifies an EntityAppearance message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an EntityAppearance message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns EntityAppearance
         */
        public static fromObject(object: { [k: string]: any }): proto.EntityAppearance;

        /**
         * Creates a plain object from an EntityAppearance message. Also converts values to other types if specified.
         * @param message EntityAppearance
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.EntityAppearance, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this EntityAppearance to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for EntityAppearance
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ChunkCoord. */
    interface IChunkCoord {

        /** ChunkCoord x */
        x?: (number|null);

        /** ChunkCoord y */
        y?: (number|null);
    }

    /** Represents a ChunkCoord. */
    class ChunkCoord implements IChunkCoord {

        /**
         * Constructs a new ChunkCoord.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IChunkCoord);

        /** ChunkCoord x. */
        public x: number;

        /** ChunkCoord y. */
        public y: number;

        /**
         * Creates a new ChunkCoord instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ChunkCoord instance
         */
        public static create(properties?: proto.IChunkCoord): proto.ChunkCoord;

        /**
         * Encodes the specified ChunkCoord message. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @param message ChunkCoord message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IChunkCoord, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ChunkCoord message, length delimited. Does not implicitly {@link proto.ChunkCoord.verify|verify} messages.
         * @param message ChunkCoord message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IChunkCoord, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.ChunkCoord;

        /**
         * Decodes a ChunkCoord message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ChunkCoord
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.ChunkCoord;

        /**
         * Verifies a ChunkCoord message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ChunkCoord message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ChunkCoord
         */
        public static fromObject(object: { [k: string]: any }): proto.ChunkCoord;

        /**
         * Creates a plain object from a ChunkCoord message. Also converts values to other types if specified.
         * @param message ChunkCoord
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.ChunkCoord, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ChunkCoord to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ChunkCoord
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ChunkData. */
    interface IChunkData {

        /** ChunkData coord */
        coord?: (proto.IChunkCoord|null);

        /** ChunkData tiles */
        tiles?: (Uint8Array|null);

        /** ChunkData version */
        version?: (number|null);
    }

    /** Represents a ChunkData. */
    class ChunkData implements IChunkData {

        /**
         * Constructs a new ChunkData.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IChunkData);

        /** ChunkData coord. */
        public coord?: (proto.IChunkCoord|null);

        /** ChunkData tiles. */
        public tiles: Uint8Array;

        /** ChunkData version. */
        public version: number;

        /**
         * Creates a new ChunkData instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ChunkData instance
         */
        public static create(properties?: proto.IChunkData): proto.ChunkData;

        /**
         * Encodes the specified ChunkData message. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @param message ChunkData message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IChunkData, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ChunkData message, length delimited. Does not implicitly {@link proto.ChunkData.verify|verify} messages.
         * @param message ChunkData message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IChunkData, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ChunkData message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.ChunkData;

        /**
         * Decodes a ChunkData message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ChunkData
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.ChunkData;

        /**
         * Verifies a ChunkData message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ChunkData message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ChunkData
         */
        public static fromObject(object: { [k: string]: any }): proto.ChunkData;

        /**
         * Creates a plain object from a ChunkData message. Also converts values to other types if specified.
         * @param message ChunkData
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.ChunkData, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ChunkData to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ChunkData
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a MoveTo. */
    interface IMoveTo {

        /** MoveTo x */
        x?: (number|null);

        /** MoveTo y */
        y?: (number|null);
    }

    /** Represents a MoveTo. */
    class MoveTo implements IMoveTo {

        /**
         * Constructs a new MoveTo.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IMoveTo);

        /** MoveTo x. */
        public x: number;

        /** MoveTo y. */
        public y: number;

        /**
         * Creates a new MoveTo instance using the specified properties.
         * @param [properties] Properties to set
         * @returns MoveTo instance
         */
        public static create(properties?: proto.IMoveTo): proto.MoveTo;

        /**
         * Encodes the specified MoveTo message. Does not implicitly {@link proto.MoveTo.verify|verify} messages.
         * @param message MoveTo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IMoveTo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified MoveTo message, length delimited. Does not implicitly {@link proto.MoveTo.verify|verify} messages.
         * @param message MoveTo message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IMoveTo, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a MoveTo message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns MoveTo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.MoveTo;

        /**
         * Decodes a MoveTo message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns MoveTo
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.MoveTo;

        /**
         * Verifies a MoveTo message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a MoveTo message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns MoveTo
         */
        public static fromObject(object: { [k: string]: any }): proto.MoveTo;

        /**
         * Creates a plain object from a MoveTo message. Also converts values to other types if specified.
         * @param message MoveTo
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.MoveTo, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this MoveTo to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for MoveTo
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a MoveToEntity. */
    interface IMoveToEntity {

        /** MoveToEntity entityId */
        entityId?: (number|Long|null);

        /** MoveToEntity autoInteract */
        autoInteract?: (boolean|null);
    }

    /** Represents a MoveToEntity. */
    class MoveToEntity implements IMoveToEntity {

        /**
         * Constructs a new MoveToEntity.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IMoveToEntity);

        /** MoveToEntity entityId. */
        public entityId: (number|Long);

        /** MoveToEntity autoInteract. */
        public autoInteract: boolean;

        /**
         * Creates a new MoveToEntity instance using the specified properties.
         * @param [properties] Properties to set
         * @returns MoveToEntity instance
         */
        public static create(properties?: proto.IMoveToEntity): proto.MoveToEntity;

        /**
         * Encodes the specified MoveToEntity message. Does not implicitly {@link proto.MoveToEntity.verify|verify} messages.
         * @param message MoveToEntity message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IMoveToEntity, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified MoveToEntity message, length delimited. Does not implicitly {@link proto.MoveToEntity.verify|verify} messages.
         * @param message MoveToEntity message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IMoveToEntity, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a MoveToEntity message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns MoveToEntity
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.MoveToEntity;

        /**
         * Decodes a MoveToEntity message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns MoveToEntity
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.MoveToEntity;

        /**
         * Verifies a MoveToEntity message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a MoveToEntity message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns MoveToEntity
         */
        public static fromObject(object: { [k: string]: any }): proto.MoveToEntity;

        /**
         * Creates a plain object from a MoveToEntity message. Also converts values to other types if specified.
         * @param message MoveToEntity
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.MoveToEntity, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this MoveToEntity to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for MoveToEntity
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of an Interact. */
    interface IInteract {

        /** Interact entityId */
        entityId?: (number|Long|null);

        /** Interact type */
        type?: (proto.InteractionType|null);
    }

    /** Represents an Interact. */
    class Interact implements IInteract {

        /**
         * Constructs a new Interact.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IInteract);

        /** Interact entityId. */
        public entityId: (number|Long);

        /** Interact type. */
        public type: proto.InteractionType;

        /**
         * Creates a new Interact instance using the specified properties.
         * @param [properties] Properties to set
         * @returns Interact instance
         */
        public static create(properties?: proto.IInteract): proto.Interact;

        /**
         * Encodes the specified Interact message. Does not implicitly {@link proto.Interact.verify|verify} messages.
         * @param message Interact message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IInteract, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified Interact message, length delimited. Does not implicitly {@link proto.Interact.verify|verify} messages.
         * @param message Interact message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IInteract, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes an Interact message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns Interact
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.Interact;

        /**
         * Decodes an Interact message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns Interact
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.Interact;

        /**
         * Verifies an Interact message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates an Interact message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns Interact
         */
        public static fromObject(object: { [k: string]: any }): proto.Interact;

        /**
         * Creates a plain object from an Interact message. Also converts values to other types if specified.
         * @param message Interact
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.Interact, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this Interact to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for Interact
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** InteractionType enum. */
    enum InteractionType {
        AUTO = 0,
        GATHER = 1,
        OPEN_CONTAINER = 2,
        CLOSE_CONTAINER = 3,
        USE = 4,
        PICKUP = 5
    }

    /** Properties of a C2S_PlayerAction. */
    interface IC2S_PlayerAction {

        /** C2S_PlayerAction moveTo */
        moveTo?: (proto.IMoveTo|null);

        /** C2S_PlayerAction moveToEntity */
        moveToEntity?: (proto.IMoveToEntity|null);

        /** C2S_PlayerAction interact */
        interact?: (proto.IInteract|null);

        /** C2S_PlayerAction modifiers */
        modifiers?: (number|null);
    }

    /** Represents a C2S_PlayerAction. */
    class C2S_PlayerAction implements IC2S_PlayerAction {

        /**
         * Constructs a new C2S_PlayerAction.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IC2S_PlayerAction);

        /** C2S_PlayerAction moveTo. */
        public moveTo?: (proto.IMoveTo|null);

        /** C2S_PlayerAction moveToEntity. */
        public moveToEntity?: (proto.IMoveToEntity|null);

        /** C2S_PlayerAction interact. */
        public interact?: (proto.IInteract|null);

        /** C2S_PlayerAction modifiers. */
        public modifiers: number;

        /** C2S_PlayerAction action. */
        public action?: ("moveTo"|"moveToEntity"|"interact");

        /**
         * Creates a new C2S_PlayerAction instance using the specified properties.
         * @param [properties] Properties to set
         * @returns C2S_PlayerAction instance
         */
        public static create(properties?: proto.IC2S_PlayerAction): proto.C2S_PlayerAction;

        /**
         * Encodes the specified C2S_PlayerAction message. Does not implicitly {@link proto.C2S_PlayerAction.verify|verify} messages.
         * @param message C2S_PlayerAction message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IC2S_PlayerAction, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified C2S_PlayerAction message, length delimited. Does not implicitly {@link proto.C2S_PlayerAction.verify|verify} messages.
         * @param message C2S_PlayerAction message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IC2S_PlayerAction, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a C2S_PlayerAction message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns C2S_PlayerAction
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.C2S_PlayerAction;

        /**
         * Decodes a C2S_PlayerAction message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns C2S_PlayerAction
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.C2S_PlayerAction;

        /**
         * Verifies a C2S_PlayerAction message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a C2S_PlayerAction message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns C2S_PlayerAction
         */
        public static fromObject(object: { [k: string]: any }): proto.C2S_PlayerAction;

        /**
         * Creates a plain object from a C2S_PlayerAction message. Also converts values to other types if specified.
         * @param message C2S_PlayerAction
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.C2S_PlayerAction, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this C2S_PlayerAction to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for C2S_PlayerAction
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a C2S_MovementMode. */
    interface IC2S_MovementMode {

        /** C2S_MovementMode mode */
        mode?: (proto.MovementMode|null);
    }

    /** Represents a C2S_MovementMode. */
    class C2S_MovementMode implements IC2S_MovementMode {

        /**
         * Constructs a new C2S_MovementMode.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IC2S_MovementMode);

        /** C2S_MovementMode mode. */
        public mode: proto.MovementMode;

        /**
         * Creates a new C2S_MovementMode instance using the specified properties.
         * @param [properties] Properties to set
         * @returns C2S_MovementMode instance
         */
        public static create(properties?: proto.IC2S_MovementMode): proto.C2S_MovementMode;

        /**
         * Encodes the specified C2S_MovementMode message. Does not implicitly {@link proto.C2S_MovementMode.verify|verify} messages.
         * @param message C2S_MovementMode message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IC2S_MovementMode, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified C2S_MovementMode message, length delimited. Does not implicitly {@link proto.C2S_MovementMode.verify|verify} messages.
         * @param message C2S_MovementMode message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IC2S_MovementMode, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a C2S_MovementMode message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns C2S_MovementMode
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.C2S_MovementMode;

        /**
         * Decodes a C2S_MovementMode message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns C2S_MovementMode
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.C2S_MovementMode;

        /**
         * Verifies a C2S_MovementMode message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a C2S_MovementMode message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns C2S_MovementMode
         */
        public static fromObject(object: { [k: string]: any }): proto.C2S_MovementMode;

        /**
         * Creates a plain object from a C2S_MovementMode message. Also converts values to other types if specified.
         * @param message C2S_MovementMode
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.C2S_MovementMode, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this C2S_MovementMode to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for C2S_MovementMode
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a C2S_Auth. */
    interface IC2S_Auth {

        /** C2S_Auth token */
        token?: (string|null);

        /** C2S_Auth clientVersion */
        clientVersion?: (string|null);
    }

    /** Represents a C2S_Auth. */
    class C2S_Auth implements IC2S_Auth {

        /**
         * Constructs a new C2S_Auth.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IC2S_Auth);

        /** C2S_Auth token. */
        public token: string;

        /** C2S_Auth clientVersion. */
        public clientVersion: string;

        /**
         * Creates a new C2S_Auth instance using the specified properties.
         * @param [properties] Properties to set
         * @returns C2S_Auth instance
         */
        public static create(properties?: proto.IC2S_Auth): proto.C2S_Auth;

        /**
         * Encodes the specified C2S_Auth message. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @param message C2S_Auth message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IC2S_Auth, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified C2S_Auth message, length delimited. Does not implicitly {@link proto.C2S_Auth.verify|verify} messages.
         * @param message C2S_Auth message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IC2S_Auth, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.C2S_Auth;

        /**
         * Decodes a C2S_Auth message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns C2S_Auth
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.C2S_Auth;

        /**
         * Verifies a C2S_Auth message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a C2S_Auth message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns C2S_Auth
         */
        public static fromObject(object: { [k: string]: any }): proto.C2S_Auth;

        /**
         * Creates a plain object from a C2S_Auth message. Also converts values to other types if specified.
         * @param message C2S_Auth
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.C2S_Auth, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this C2S_Auth to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for C2S_Auth
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a C2S_Ping. */
    interface IC2S_Ping {

        /** C2S_Ping clientTimeMs */
        clientTimeMs?: (number|Long|null);
    }

    /** Represents a C2S_Ping. */
    class C2S_Ping implements IC2S_Ping {

        /**
         * Constructs a new C2S_Ping.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IC2S_Ping);

        /** C2S_Ping clientTimeMs. */
        public clientTimeMs: (number|Long);

        /**
         * Creates a new C2S_Ping instance using the specified properties.
         * @param [properties] Properties to set
         * @returns C2S_Ping instance
         */
        public static create(properties?: proto.IC2S_Ping): proto.C2S_Ping;

        /**
         * Encodes the specified C2S_Ping message. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @param message C2S_Ping message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IC2S_Ping, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified C2S_Ping message, length delimited. Does not implicitly {@link proto.C2S_Ping.verify|verify} messages.
         * @param message C2S_Ping message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IC2S_Ping, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.C2S_Ping;

        /**
         * Decodes a C2S_Ping message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns C2S_Ping
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.C2S_Ping;

        /**
         * Verifies a C2S_Ping message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a C2S_Ping message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns C2S_Ping
         */
        public static fromObject(object: { [k: string]: any }): proto.C2S_Ping;

        /**
         * Creates a plain object from a C2S_Ping message. Also converts values to other types if specified.
         * @param message C2S_Ping
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.C2S_Ping, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this C2S_Ping to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for C2S_Ping
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ClientMessage. */
    interface IClientMessage {

        /** ClientMessage sequence */
        sequence?: (number|null);

        /** ClientMessage auth */
        auth?: (proto.IC2S_Auth|null);

        /** ClientMessage ping */
        ping?: (proto.IC2S_Ping|null);

        /** ClientMessage playerAction */
        playerAction?: (proto.IC2S_PlayerAction|null);

        /** ClientMessage movementMode */
        movementMode?: (proto.IC2S_MovementMode|null);

        /** ClientMessage inventoryOp */
        inventoryOp?: (proto.IC2S_InventoryOp|null);
    }

    /** Represents a ClientMessage. */
    class ClientMessage implements IClientMessage {

        /**
         * Constructs a new ClientMessage.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IClientMessage);

        /** ClientMessage sequence. */
        public sequence: number;

        /** ClientMessage auth. */
        public auth?: (proto.IC2S_Auth|null);

        /** ClientMessage ping. */
        public ping?: (proto.IC2S_Ping|null);

        /** ClientMessage playerAction. */
        public playerAction?: (proto.IC2S_PlayerAction|null);

        /** ClientMessage movementMode. */
        public movementMode?: (proto.IC2S_MovementMode|null);

        /** ClientMessage inventoryOp. */
        public inventoryOp?: (proto.IC2S_InventoryOp|null);

        /** ClientMessage payload. */
        public payload?: ("auth"|"ping"|"playerAction"|"movementMode"|"inventoryOp");

        /**
         * Creates a new ClientMessage instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ClientMessage instance
         */
        public static create(properties?: proto.IClientMessage): proto.ClientMessage;

        /**
         * Encodes the specified ClientMessage message. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @param message ClientMessage message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IClientMessage, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ClientMessage message, length delimited. Does not implicitly {@link proto.ClientMessage.verify|verify} messages.
         * @param message ClientMessage message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IClientMessage, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ClientMessage message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.ClientMessage;

        /**
         * Decodes a ClientMessage message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ClientMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.ClientMessage;

        /**
         * Verifies a ClientMessage message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ClientMessage message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ClientMessage
         */
        public static fromObject(object: { [k: string]: any }): proto.ClientMessage;

        /**
         * Creates a plain object from a ClientMessage message. Also converts values to other types if specified.
         * @param message ClientMessage
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.ClientMessage, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ClientMessage to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ClientMessage
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_AuthResult. */
    interface IS2C_AuthResult {

        /** S2C_AuthResult success */
        success?: (boolean|null);

        /** S2C_AuthResult errorMessage */
        errorMessage?: (string|null);
    }

    /** Represents a S2C_AuthResult. */
    class S2C_AuthResult implements IS2C_AuthResult {

        /**
         * Constructs a new S2C_AuthResult.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_AuthResult);

        /** S2C_AuthResult success. */
        public success: boolean;

        /** S2C_AuthResult errorMessage. */
        public errorMessage: string;

        /**
         * Creates a new S2C_AuthResult instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_AuthResult instance
         */
        public static create(properties?: proto.IS2C_AuthResult): proto.S2C_AuthResult;

        /**
         * Encodes the specified S2C_AuthResult message. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @param message S2C_AuthResult message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_AuthResult, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_AuthResult message, length delimited. Does not implicitly {@link proto.S2C_AuthResult.verify|verify} messages.
         * @param message S2C_AuthResult message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_AuthResult, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_AuthResult;

        /**
         * Decodes a S2C_AuthResult message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_AuthResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_AuthResult;

        /**
         * Verifies a S2C_AuthResult message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_AuthResult message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_AuthResult
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_AuthResult;

        /**
         * Creates a plain object from a S2C_AuthResult message. Also converts values to other types if specified.
         * @param message S2C_AuthResult
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_AuthResult, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_AuthResult to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_AuthResult
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_Pong. */
    interface IS2C_Pong {

        /** S2C_Pong clientTimeMs */
        clientTimeMs?: (number|Long|null);

        /** S2C_Pong serverTimeMs */
        serverTimeMs?: (number|Long|null);
    }

    /** Represents a S2C_Pong. */
    class S2C_Pong implements IS2C_Pong {

        /**
         * Constructs a new S2C_Pong.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_Pong);

        /** S2C_Pong clientTimeMs. */
        public clientTimeMs: (number|Long);

        /** S2C_Pong serverTimeMs. */
        public serverTimeMs: (number|Long);

        /**
         * Creates a new S2C_Pong instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_Pong instance
         */
        public static create(properties?: proto.IS2C_Pong): proto.S2C_Pong;

        /**
         * Encodes the specified S2C_Pong message. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @param message S2C_Pong message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_Pong, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_Pong message, length delimited. Does not implicitly {@link proto.S2C_Pong.verify|verify} messages.
         * @param message S2C_Pong message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_Pong, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_Pong;

        /**
         * Decodes a S2C_Pong message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_Pong
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_Pong;

        /**
         * Verifies a S2C_Pong message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_Pong message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_Pong
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_Pong;

        /**
         * Creates a plain object from a S2C_Pong message. Also converts values to other types if specified.
         * @param message S2C_Pong
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_Pong, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_Pong to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_Pong
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_PlayerEnterWorld. */
    interface IS2C_PlayerEnterWorld {

        /** S2C_PlayerEnterWorld entityId */
        entityId?: (number|Long|null);

        /** S2C_PlayerEnterWorld name */
        name?: (string|null);

        /** S2C_PlayerEnterWorld coordPerTile */
        coordPerTile?: (number|null);

        /** S2C_PlayerEnterWorld chunkSize */
        chunkSize?: (number|null);

        /** S2C_PlayerEnterWorld tickRate */
        tickRate?: (number|null);

        /** S2C_PlayerEnterWorld streamEpoch */
        streamEpoch?: (number|null);
    }

    /** Represents a S2C_PlayerEnterWorld. */
    class S2C_PlayerEnterWorld implements IS2C_PlayerEnterWorld {

        /**
         * Constructs a new S2C_PlayerEnterWorld.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_PlayerEnterWorld);

        /** S2C_PlayerEnterWorld entityId. */
        public entityId: (number|Long);

        /** S2C_PlayerEnterWorld name. */
        public name: string;

        /** S2C_PlayerEnterWorld coordPerTile. */
        public coordPerTile: number;

        /** S2C_PlayerEnterWorld chunkSize. */
        public chunkSize: number;

        /** S2C_PlayerEnterWorld tickRate. */
        public tickRate: number;

        /** S2C_PlayerEnterWorld streamEpoch. */
        public streamEpoch: number;

        /**
         * Creates a new S2C_PlayerEnterWorld instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_PlayerEnterWorld instance
         */
        public static create(properties?: proto.IS2C_PlayerEnterWorld): proto.S2C_PlayerEnterWorld;

        /**
         * Encodes the specified S2C_PlayerEnterWorld message. Does not implicitly {@link proto.S2C_PlayerEnterWorld.verify|verify} messages.
         * @param message S2C_PlayerEnterWorld message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_PlayerEnterWorld, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_PlayerEnterWorld message, length delimited. Does not implicitly {@link proto.S2C_PlayerEnterWorld.verify|verify} messages.
         * @param message S2C_PlayerEnterWorld message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_PlayerEnterWorld, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_PlayerEnterWorld message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_PlayerEnterWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_PlayerEnterWorld;

        /**
         * Decodes a S2C_PlayerEnterWorld message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_PlayerEnterWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_PlayerEnterWorld;

        /**
         * Verifies a S2C_PlayerEnterWorld message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_PlayerEnterWorld message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_PlayerEnterWorld
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_PlayerEnterWorld;

        /**
         * Creates a plain object from a S2C_PlayerEnterWorld message. Also converts values to other types if specified.
         * @param message S2C_PlayerEnterWorld
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_PlayerEnterWorld, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_PlayerEnterWorld to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_PlayerEnterWorld
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_PlayerLeaveWorld. */
    interface IS2C_PlayerLeaveWorld {

        /** S2C_PlayerLeaveWorld entityId */
        entityId?: (number|Long|null);
    }

    /** Represents a S2C_PlayerLeaveWorld. */
    class S2C_PlayerLeaveWorld implements IS2C_PlayerLeaveWorld {

        /**
         * Constructs a new S2C_PlayerLeaveWorld.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_PlayerLeaveWorld);

        /** S2C_PlayerLeaveWorld entityId. */
        public entityId: (number|Long);

        /**
         * Creates a new S2C_PlayerLeaveWorld instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_PlayerLeaveWorld instance
         */
        public static create(properties?: proto.IS2C_PlayerLeaveWorld): proto.S2C_PlayerLeaveWorld;

        /**
         * Encodes the specified S2C_PlayerLeaveWorld message. Does not implicitly {@link proto.S2C_PlayerLeaveWorld.verify|verify} messages.
         * @param message S2C_PlayerLeaveWorld message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_PlayerLeaveWorld, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_PlayerLeaveWorld message, length delimited. Does not implicitly {@link proto.S2C_PlayerLeaveWorld.verify|verify} messages.
         * @param message S2C_PlayerLeaveWorld message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_PlayerLeaveWorld, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_PlayerLeaveWorld message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_PlayerLeaveWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_PlayerLeaveWorld;

        /**
         * Decodes a S2C_PlayerLeaveWorld message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_PlayerLeaveWorld
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_PlayerLeaveWorld;

        /**
         * Verifies a S2C_PlayerLeaveWorld message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_PlayerLeaveWorld message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_PlayerLeaveWorld
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_PlayerLeaveWorld;

        /**
         * Creates a plain object from a S2C_PlayerLeaveWorld message. Also converts values to other types if specified.
         * @param message S2C_PlayerLeaveWorld
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_PlayerLeaveWorld, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_PlayerLeaveWorld to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_PlayerLeaveWorld
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ChunkLoad. */
    interface IS2C_ChunkLoad {

        /** S2C_ChunkLoad chunk */
        chunk?: (proto.IChunkData|null);
    }

    /** Represents a S2C_ChunkLoad. */
    class S2C_ChunkLoad implements IS2C_ChunkLoad {

        /**
         * Constructs a new S2C_ChunkLoad.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ChunkLoad);

        /** S2C_ChunkLoad chunk. */
        public chunk?: (proto.IChunkData|null);

        /**
         * Creates a new S2C_ChunkLoad instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ChunkLoad instance
         */
        public static create(properties?: proto.IS2C_ChunkLoad): proto.S2C_ChunkLoad;

        /**
         * Encodes the specified S2C_ChunkLoad message. Does not implicitly {@link proto.S2C_ChunkLoad.verify|verify} messages.
         * @param message S2C_ChunkLoad message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ChunkLoad, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ChunkLoad message, length delimited. Does not implicitly {@link proto.S2C_ChunkLoad.verify|verify} messages.
         * @param message S2C_ChunkLoad message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ChunkLoad, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ChunkLoad message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ChunkLoad
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ChunkLoad;

        /**
         * Decodes a S2C_ChunkLoad message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ChunkLoad
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ChunkLoad;

        /**
         * Verifies a S2C_ChunkLoad message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ChunkLoad message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ChunkLoad
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ChunkLoad;

        /**
         * Creates a plain object from a S2C_ChunkLoad message. Also converts values to other types if specified.
         * @param message S2C_ChunkLoad
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ChunkLoad, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ChunkLoad to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ChunkLoad
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ChunkUnload. */
    interface IS2C_ChunkUnload {

        /** S2C_ChunkUnload coord */
        coord?: (proto.IChunkCoord|null);
    }

    /** Represents a S2C_ChunkUnload. */
    class S2C_ChunkUnload implements IS2C_ChunkUnload {

        /**
         * Constructs a new S2C_ChunkUnload.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ChunkUnload);

        /** S2C_ChunkUnload coord. */
        public coord?: (proto.IChunkCoord|null);

        /**
         * Creates a new S2C_ChunkUnload instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ChunkUnload instance
         */
        public static create(properties?: proto.IS2C_ChunkUnload): proto.S2C_ChunkUnload;

        /**
         * Encodes the specified S2C_ChunkUnload message. Does not implicitly {@link proto.S2C_ChunkUnload.verify|verify} messages.
         * @param message S2C_ChunkUnload message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ChunkUnload, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ChunkUnload message, length delimited. Does not implicitly {@link proto.S2C_ChunkUnload.verify|verify} messages.
         * @param message S2C_ChunkUnload message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ChunkUnload, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ChunkUnload message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ChunkUnload
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ChunkUnload;

        /**
         * Decodes a S2C_ChunkUnload message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ChunkUnload
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ChunkUnload;

        /**
         * Verifies a S2C_ChunkUnload message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ChunkUnload message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ChunkUnload
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ChunkUnload;

        /**
         * Creates a plain object from a S2C_ChunkUnload message. Also converts values to other types if specified.
         * @param message S2C_ChunkUnload
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ChunkUnload, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ChunkUnload to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ChunkUnload
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ObjectSpawn. */
    interface IS2C_ObjectSpawn {

        /** S2C_ObjectSpawn entityId */
        entityId?: (number|Long|null);

        /** S2C_ObjectSpawn objectType */
        objectType?: (number|null);

        /** S2C_ObjectSpawn resourcePath */
        resourcePath?: (string|null);

        /** S2C_ObjectSpawn position */
        position?: (proto.IEntityPosition|null);
    }

    /** Represents a S2C_ObjectSpawn. */
    class S2C_ObjectSpawn implements IS2C_ObjectSpawn {

        /**
         * Constructs a new S2C_ObjectSpawn.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ObjectSpawn);

        /** S2C_ObjectSpawn entityId. */
        public entityId: (number|Long);

        /** S2C_ObjectSpawn objectType. */
        public objectType: number;

        /** S2C_ObjectSpawn resourcePath. */
        public resourcePath: string;

        /** S2C_ObjectSpawn position. */
        public position?: (proto.IEntityPosition|null);

        /**
         * Creates a new S2C_ObjectSpawn instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ObjectSpawn instance
         */
        public static create(properties?: proto.IS2C_ObjectSpawn): proto.S2C_ObjectSpawn;

        /**
         * Encodes the specified S2C_ObjectSpawn message. Does not implicitly {@link proto.S2C_ObjectSpawn.verify|verify} messages.
         * @param message S2C_ObjectSpawn message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ObjectSpawn, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ObjectSpawn message, length delimited. Does not implicitly {@link proto.S2C_ObjectSpawn.verify|verify} messages.
         * @param message S2C_ObjectSpawn message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ObjectSpawn, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ObjectSpawn message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ObjectSpawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ObjectSpawn;

        /**
         * Decodes a S2C_ObjectSpawn message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ObjectSpawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ObjectSpawn;

        /**
         * Verifies a S2C_ObjectSpawn message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ObjectSpawn message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ObjectSpawn
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ObjectSpawn;

        /**
         * Creates a plain object from a S2C_ObjectSpawn message. Also converts values to other types if specified.
         * @param message S2C_ObjectSpawn
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ObjectSpawn, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ObjectSpawn to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ObjectSpawn
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ObjectDespawn. */
    interface IS2C_ObjectDespawn {

        /** S2C_ObjectDespawn entityId */
        entityId?: (number|Long|null);
    }

    /** Represents a S2C_ObjectDespawn. */
    class S2C_ObjectDespawn implements IS2C_ObjectDespawn {

        /**
         * Constructs a new S2C_ObjectDespawn.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ObjectDespawn);

        /** S2C_ObjectDespawn entityId. */
        public entityId: (number|Long);

        /**
         * Creates a new S2C_ObjectDespawn instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ObjectDespawn instance
         */
        public static create(properties?: proto.IS2C_ObjectDespawn): proto.S2C_ObjectDespawn;

        /**
         * Encodes the specified S2C_ObjectDespawn message. Does not implicitly {@link proto.S2C_ObjectDespawn.verify|verify} messages.
         * @param message S2C_ObjectDespawn message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ObjectDespawn, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ObjectDespawn message, length delimited. Does not implicitly {@link proto.S2C_ObjectDespawn.verify|verify} messages.
         * @param message S2C_ObjectDespawn message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ObjectDespawn, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ObjectDespawn message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ObjectDespawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ObjectDespawn;

        /**
         * Decodes a S2C_ObjectDespawn message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ObjectDespawn
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ObjectDespawn;

        /**
         * Verifies a S2C_ObjectDespawn message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ObjectDespawn message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ObjectDespawn
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ObjectDespawn;

        /**
         * Creates a plain object from a S2C_ObjectDespawn message. Also converts values to other types if specified.
         * @param message S2C_ObjectDespawn
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ObjectDespawn, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ObjectDespawn to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ObjectDespawn
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ObjectMove. */
    interface IS2C_ObjectMove {

        /** S2C_ObjectMove entityId */
        entityId?: (number|Long|null);

        /** S2C_ObjectMove movement */
        movement?: (proto.IEntityMovement|null);

        /** S2C_ObjectMove serverTimeMs */
        serverTimeMs?: (number|Long|null);

        /** S2C_ObjectMove moveSeq */
        moveSeq?: (number|null);

        /** S2C_ObjectMove isTeleport */
        isTeleport?: (boolean|null);

        /** S2C_ObjectMove streamEpoch */
        streamEpoch?: (number|null);
    }

    /** Represents a S2C_ObjectMove. */
    class S2C_ObjectMove implements IS2C_ObjectMove {

        /**
         * Constructs a new S2C_ObjectMove.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ObjectMove);

        /** S2C_ObjectMove entityId. */
        public entityId: (number|Long);

        /** S2C_ObjectMove movement. */
        public movement?: (proto.IEntityMovement|null);

        /** S2C_ObjectMove serverTimeMs. */
        public serverTimeMs: (number|Long);

        /** S2C_ObjectMove moveSeq. */
        public moveSeq: number;

        /** S2C_ObjectMove isTeleport. */
        public isTeleport: boolean;

        /** S2C_ObjectMove streamEpoch. */
        public streamEpoch: number;

        /**
         * Creates a new S2C_ObjectMove instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ObjectMove instance
         */
        public static create(properties?: proto.IS2C_ObjectMove): proto.S2C_ObjectMove;

        /**
         * Encodes the specified S2C_ObjectMove message. Does not implicitly {@link proto.S2C_ObjectMove.verify|verify} messages.
         * @param message S2C_ObjectMove message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ObjectMove, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ObjectMove message, length delimited. Does not implicitly {@link proto.S2C_ObjectMove.verify|verify} messages.
         * @param message S2C_ObjectMove message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ObjectMove, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ObjectMove message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ObjectMove
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ObjectMove;

        /**
         * Decodes a S2C_ObjectMove message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ObjectMove
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ObjectMove;

        /**
         * Verifies a S2C_ObjectMove message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ObjectMove message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ObjectMove
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ObjectMove;

        /**
         * Creates a plain object from a S2C_ObjectMove message. Also converts values to other types if specified.
         * @param message S2C_ObjectMove
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ObjectMove, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ObjectMove to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ObjectMove
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_InventoryOpResult. */
    interface IS2C_InventoryOpResult {

        /** S2C_InventoryOpResult opId */
        opId?: (number|Long|null);

        /** S2C_InventoryOpResult success */
        success?: (boolean|null);

        /** S2C_InventoryOpResult error */
        error?: (proto.ErrorCode|null);

        /** S2C_InventoryOpResult message */
        message?: (string|null);

        /** S2C_InventoryOpResult updated */
        updated?: (proto.IInventoryState[]|null);

        /** S2C_InventoryOpResult spawnedDroppedEntityId */
        spawnedDroppedEntityId?: (number|Long|null);

        /** S2C_InventoryOpResult despawnedDroppedEntityId */
        despawnedDroppedEntityId?: (number|Long|null);
    }

    /** Represents a S2C_InventoryOpResult. */
    class S2C_InventoryOpResult implements IS2C_InventoryOpResult {

        /**
         * Constructs a new S2C_InventoryOpResult.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_InventoryOpResult);

        /** S2C_InventoryOpResult opId. */
        public opId: (number|Long);

        /** S2C_InventoryOpResult success. */
        public success: boolean;

        /** S2C_InventoryOpResult error. */
        public error: proto.ErrorCode;

        /** S2C_InventoryOpResult message. */
        public message: string;

        /** S2C_InventoryOpResult updated. */
        public updated: proto.IInventoryState[];

        /** S2C_InventoryOpResult spawnedDroppedEntityId. */
        public spawnedDroppedEntityId?: (number|Long|null);

        /** S2C_InventoryOpResult despawnedDroppedEntityId. */
        public despawnedDroppedEntityId?: (number|Long|null);

        /**
         * Creates a new S2C_InventoryOpResult instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_InventoryOpResult instance
         */
        public static create(properties?: proto.IS2C_InventoryOpResult): proto.S2C_InventoryOpResult;

        /**
         * Encodes the specified S2C_InventoryOpResult message. Does not implicitly {@link proto.S2C_InventoryOpResult.verify|verify} messages.
         * @param message S2C_InventoryOpResult message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_InventoryOpResult, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_InventoryOpResult message, length delimited. Does not implicitly {@link proto.S2C_InventoryOpResult.verify|verify} messages.
         * @param message S2C_InventoryOpResult message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_InventoryOpResult, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_InventoryOpResult message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_InventoryOpResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_InventoryOpResult;

        /**
         * Decodes a S2C_InventoryOpResult message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_InventoryOpResult
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_InventoryOpResult;

        /**
         * Verifies a S2C_InventoryOpResult message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_InventoryOpResult message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_InventoryOpResult
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_InventoryOpResult;

        /**
         * Creates a plain object from a S2C_InventoryOpResult message. Also converts values to other types if specified.
         * @param message S2C_InventoryOpResult
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_InventoryOpResult, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_InventoryOpResult to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_InventoryOpResult
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_InventoryUpdate. */
    interface IS2C_InventoryUpdate {

        /** S2C_InventoryUpdate updated */
        updated?: (proto.IInventoryState[]|null);
    }

    /** Represents a S2C_InventoryUpdate. */
    class S2C_InventoryUpdate implements IS2C_InventoryUpdate {

        /**
         * Constructs a new S2C_InventoryUpdate.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_InventoryUpdate);

        /** S2C_InventoryUpdate updated. */
        public updated: proto.IInventoryState[];

        /**
         * Creates a new S2C_InventoryUpdate instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_InventoryUpdate instance
         */
        public static create(properties?: proto.IS2C_InventoryUpdate): proto.S2C_InventoryUpdate;

        /**
         * Encodes the specified S2C_InventoryUpdate message. Does not implicitly {@link proto.S2C_InventoryUpdate.verify|verify} messages.
         * @param message S2C_InventoryUpdate message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_InventoryUpdate, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_InventoryUpdate message, length delimited. Does not implicitly {@link proto.S2C_InventoryUpdate.verify|verify} messages.
         * @param message S2C_InventoryUpdate message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_InventoryUpdate, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_InventoryUpdate message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_InventoryUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_InventoryUpdate;

        /**
         * Decodes a S2C_InventoryUpdate message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_InventoryUpdate
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_InventoryUpdate;

        /**
         * Verifies a S2C_InventoryUpdate message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_InventoryUpdate message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_InventoryUpdate
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_InventoryUpdate;

        /**
         * Creates a plain object from a S2C_InventoryUpdate message. Also converts values to other types if specified.
         * @param message S2C_InventoryUpdate
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_InventoryUpdate, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_InventoryUpdate to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_InventoryUpdate
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ContainerOpened. */
    interface IS2C_ContainerOpened {

        /** S2C_ContainerOpened state */
        state?: (proto.IInventoryState|null);
    }

    /** Represents a S2C_ContainerOpened. */
    class S2C_ContainerOpened implements IS2C_ContainerOpened {

        /**
         * Constructs a new S2C_ContainerOpened.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ContainerOpened);

        /** S2C_ContainerOpened state. */
        public state?: (proto.IInventoryState|null);

        /**
         * Creates a new S2C_ContainerOpened instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ContainerOpened instance
         */
        public static create(properties?: proto.IS2C_ContainerOpened): proto.S2C_ContainerOpened;

        /**
         * Encodes the specified S2C_ContainerOpened message. Does not implicitly {@link proto.S2C_ContainerOpened.verify|verify} messages.
         * @param message S2C_ContainerOpened message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ContainerOpened, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ContainerOpened message, length delimited. Does not implicitly {@link proto.S2C_ContainerOpened.verify|verify} messages.
         * @param message S2C_ContainerOpened message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ContainerOpened, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ContainerOpened message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ContainerOpened
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ContainerOpened;

        /**
         * Decodes a S2C_ContainerOpened message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ContainerOpened
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ContainerOpened;

        /**
         * Verifies a S2C_ContainerOpened message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ContainerOpened message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ContainerOpened
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ContainerOpened;

        /**
         * Creates a plain object from a S2C_ContainerOpened message. Also converts values to other types if specified.
         * @param message S2C_ContainerOpened
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ContainerOpened, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ContainerOpened to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ContainerOpened
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_ContainerClosed. */
    interface IS2C_ContainerClosed {

        /** S2C_ContainerClosed entityId */
        entityId?: (number|Long|null);
    }

    /** Represents a S2C_ContainerClosed. */
    class S2C_ContainerClosed implements IS2C_ContainerClosed {

        /**
         * Constructs a new S2C_ContainerClosed.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_ContainerClosed);

        /** S2C_ContainerClosed entityId. */
        public entityId: (number|Long);

        /**
         * Creates a new S2C_ContainerClosed instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_ContainerClosed instance
         */
        public static create(properties?: proto.IS2C_ContainerClosed): proto.S2C_ContainerClosed;

        /**
         * Encodes the specified S2C_ContainerClosed message. Does not implicitly {@link proto.S2C_ContainerClosed.verify|verify} messages.
         * @param message S2C_ContainerClosed message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_ContainerClosed, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_ContainerClosed message, length delimited. Does not implicitly {@link proto.S2C_ContainerClosed.verify|verify} messages.
         * @param message S2C_ContainerClosed message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_ContainerClosed, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_ContainerClosed message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_ContainerClosed
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_ContainerClosed;

        /**
         * Decodes a S2C_ContainerClosed message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_ContainerClosed
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_ContainerClosed;

        /**
         * Verifies a S2C_ContainerClosed message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_ContainerClosed message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_ContainerClosed
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_ContainerClosed;

        /**
         * Creates a plain object from a S2C_ContainerClosed message. Also converts values to other types if specified.
         * @param message S2C_ContainerClosed
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_ContainerClosed, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_ContainerClosed to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_ContainerClosed
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_Error. */
    interface IS2C_Error {

        /** S2C_Error code */
        code?: (proto.ErrorCode|null);

        /** S2C_Error message */
        message?: (string|null);
    }

    /** Represents a S2C_Error. */
    class S2C_Error implements IS2C_Error {

        /**
         * Constructs a new S2C_Error.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_Error);

        /** S2C_Error code. */
        public code: proto.ErrorCode;

        /** S2C_Error message. */
        public message: string;

        /**
         * Creates a new S2C_Error instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_Error instance
         */
        public static create(properties?: proto.IS2C_Error): proto.S2C_Error;

        /**
         * Encodes the specified S2C_Error message. Does not implicitly {@link proto.S2C_Error.verify|verify} messages.
         * @param message S2C_Error message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_Error, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_Error message, length delimited. Does not implicitly {@link proto.S2C_Error.verify|verify} messages.
         * @param message S2C_Error message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_Error, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_Error message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_Error
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_Error;

        /**
         * Decodes a S2C_Error message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_Error
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_Error;

        /**
         * Verifies a S2C_Error message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_Error message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_Error
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_Error;

        /**
         * Creates a plain object from a S2C_Error message. Also converts values to other types if specified.
         * @param message S2C_Error
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_Error, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_Error to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_Error
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a S2C_Warning. */
    interface IS2C_Warning {

        /** S2C_Warning code */
        code?: (proto.WarningCode|null);

        /** S2C_Warning message */
        message?: (string|null);
    }

    /** Represents a S2C_Warning. */
    class S2C_Warning implements IS2C_Warning {

        /**
         * Constructs a new S2C_Warning.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IS2C_Warning);

        /** S2C_Warning code. */
        public code: proto.WarningCode;

        /** S2C_Warning message. */
        public message: string;

        /**
         * Creates a new S2C_Warning instance using the specified properties.
         * @param [properties] Properties to set
         * @returns S2C_Warning instance
         */
        public static create(properties?: proto.IS2C_Warning): proto.S2C_Warning;

        /**
         * Encodes the specified S2C_Warning message. Does not implicitly {@link proto.S2C_Warning.verify|verify} messages.
         * @param message S2C_Warning message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IS2C_Warning, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified S2C_Warning message, length delimited. Does not implicitly {@link proto.S2C_Warning.verify|verify} messages.
         * @param message S2C_Warning message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IS2C_Warning, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a S2C_Warning message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns S2C_Warning
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.S2C_Warning;

        /**
         * Decodes a S2C_Warning message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns S2C_Warning
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.S2C_Warning;

        /**
         * Verifies a S2C_Warning message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a S2C_Warning message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns S2C_Warning
         */
        public static fromObject(object: { [k: string]: any }): proto.S2C_Warning;

        /**
         * Creates a plain object from a S2C_Warning message. Also converts values to other types if specified.
         * @param message S2C_Warning
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.S2C_Warning, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this S2C_Warning to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for S2C_Warning
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }

    /** Properties of a ServerMessage. */
    interface IServerMessage {

        /** ServerMessage sequence */
        sequence?: (number|null);

        /** ServerMessage authResult */
        authResult?: (proto.IS2C_AuthResult|null);

        /** ServerMessage pong */
        pong?: (proto.IS2C_Pong|null);

        /** ServerMessage chunkLoad */
        chunkLoad?: (proto.IS2C_ChunkLoad|null);

        /** ServerMessage chunkUnload */
        chunkUnload?: (proto.IS2C_ChunkUnload|null);

        /** ServerMessage playerEnterWorld */
        playerEnterWorld?: (proto.IS2C_PlayerEnterWorld|null);

        /** ServerMessage playerLeaveWorld */
        playerLeaveWorld?: (proto.IS2C_PlayerLeaveWorld|null);

        /** ServerMessage objectSpawn */
        objectSpawn?: (proto.IS2C_ObjectSpawn|null);

        /** ServerMessage objectDespawn */
        objectDespawn?: (proto.IS2C_ObjectDespawn|null);

        /** ServerMessage objectMove */
        objectMove?: (proto.IS2C_ObjectMove|null);

        /** ServerMessage inventoryOpResult */
        inventoryOpResult?: (proto.IS2C_InventoryOpResult|null);

        /** ServerMessage inventoryUpdate */
        inventoryUpdate?: (proto.IS2C_InventoryUpdate|null);

        /** ServerMessage containerOpened */
        containerOpened?: (proto.IS2C_ContainerOpened|null);

        /** ServerMessage containerClosed */
        containerClosed?: (proto.IS2C_ContainerClosed|null);

        /** ServerMessage error */
        error?: (proto.IS2C_Error|null);

        /** ServerMessage warning */
        warning?: (proto.IS2C_Warning|null);
    }

    /** Represents a ServerMessage. */
    class ServerMessage implements IServerMessage {

        /**
         * Constructs a new ServerMessage.
         * @param [properties] Properties to set
         */
        constructor(properties?: proto.IServerMessage);

        /** ServerMessage sequence. */
        public sequence: number;

        /** ServerMessage authResult. */
        public authResult?: (proto.IS2C_AuthResult|null);

        /** ServerMessage pong. */
        public pong?: (proto.IS2C_Pong|null);

        /** ServerMessage chunkLoad. */
        public chunkLoad?: (proto.IS2C_ChunkLoad|null);

        /** ServerMessage chunkUnload. */
        public chunkUnload?: (proto.IS2C_ChunkUnload|null);

        /** ServerMessage playerEnterWorld. */
        public playerEnterWorld?: (proto.IS2C_PlayerEnterWorld|null);

        /** ServerMessage playerLeaveWorld. */
        public playerLeaveWorld?: (proto.IS2C_PlayerLeaveWorld|null);

        /** ServerMessage objectSpawn. */
        public objectSpawn?: (proto.IS2C_ObjectSpawn|null);

        /** ServerMessage objectDespawn. */
        public objectDespawn?: (proto.IS2C_ObjectDespawn|null);

        /** ServerMessage objectMove. */
        public objectMove?: (proto.IS2C_ObjectMove|null);

        /** ServerMessage inventoryOpResult. */
        public inventoryOpResult?: (proto.IS2C_InventoryOpResult|null);

        /** ServerMessage inventoryUpdate. */
        public inventoryUpdate?: (proto.IS2C_InventoryUpdate|null);

        /** ServerMessage containerOpened. */
        public containerOpened?: (proto.IS2C_ContainerOpened|null);

        /** ServerMessage containerClosed. */
        public containerClosed?: (proto.IS2C_ContainerClosed|null);

        /** ServerMessage error. */
        public error?: (proto.IS2C_Error|null);

        /** ServerMessage warning. */
        public warning?: (proto.IS2C_Warning|null);

        /** ServerMessage payload. */
        public payload?: ("authResult"|"pong"|"chunkLoad"|"chunkUnload"|"playerEnterWorld"|"playerLeaveWorld"|"objectSpawn"|"objectDespawn"|"objectMove"|"inventoryOpResult"|"inventoryUpdate"|"containerOpened"|"containerClosed"|"error"|"warning");

        /**
         * Creates a new ServerMessage instance using the specified properties.
         * @param [properties] Properties to set
         * @returns ServerMessage instance
         */
        public static create(properties?: proto.IServerMessage): proto.ServerMessage;

        /**
         * Encodes the specified ServerMessage message. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @param message ServerMessage message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encode(message: proto.IServerMessage, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Encodes the specified ServerMessage message, length delimited. Does not implicitly {@link proto.ServerMessage.verify|verify} messages.
         * @param message ServerMessage message or plain object to encode
         * @param [writer] Writer to encode to
         * @returns Writer
         */
        public static encodeDelimited(message: proto.IServerMessage, writer?: $protobuf.Writer): $protobuf.Writer;

        /**
         * Decodes a ServerMessage message from the specified reader or buffer.
         * @param reader Reader or buffer to decode from
         * @param [length] Message length if known beforehand
         * @returns ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): proto.ServerMessage;

        /**
         * Decodes a ServerMessage message from the specified reader or buffer, length delimited.
         * @param reader Reader or buffer to decode from
         * @returns ServerMessage
         * @throws {Error} If the payload is not a reader or valid buffer
         * @throws {$protobuf.util.ProtocolError} If required fields are missing
         */
        public static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): proto.ServerMessage;

        /**
         * Verifies a ServerMessage message.
         * @param message Plain object to verify
         * @returns `null` if valid, otherwise the reason why it is not
         */
        public static verify(message: { [k: string]: any }): (string|null);

        /**
         * Creates a ServerMessage message from a plain object. Also converts values to their respective internal types.
         * @param object Plain object
         * @returns ServerMessage
         */
        public static fromObject(object: { [k: string]: any }): proto.ServerMessage;

        /**
         * Creates a plain object from a ServerMessage message. Also converts values to other types if specified.
         * @param message ServerMessage
         * @param [options] Conversion options
         * @returns Plain object
         */
        public static toObject(message: proto.ServerMessage, options?: $protobuf.IConversionOptions): { [k: string]: any };

        /**
         * Converts this ServerMessage to JSON.
         * @returns JSON object
         */
        public toJSON(): { [k: string]: any };

        /**
         * Gets the default type url for ServerMessage
         * @param [typeUrlPrefix] your custom typeUrlPrefix(default "type.googleapis.com")
         * @returns The default type url
         */
        public static getTypeUrl(typeUrlPrefix?: string): string;
    }
}
